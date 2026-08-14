package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Client struct {
	Addr string
	User string

	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	PrivateKey, Certificate string

	Client *ssh.Client
	conn   ssh.Conn
}

func (c *Client) Close() error {
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
	}

	c.conn = nil

	return nil
}

type connResp struct {
	err    error
	conn   ssh.Conn
	client *ssh.Client
}

func (c *Client) Connect(ctx context.Context) error {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.Certificate))
	if err != nil {
		return err
	}

	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return errors.New("SSH public key must be a certificate")
	}

	keySigner, err := ssh.ParsePrivateKey([]byte(c.PrivateKey))
	if err != nil {
		return err
	}

	signer, err := ssh.NewCertSigner(cert, keySigner)
	if err != nil {
		log.Fatal(err)
	}

	tcpConn, err := c.Dial(ctx, "tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	conf := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
	}

	respCh := make(chan connResp)

	// ssh.NewClientConn doesn't take a context, so we need to handle cancelation on our end
	go func() {
		conn, chans, reqs, err := ssh.NewClientConn(tcpConn, tcpConn.RemoteAddr().String(), conf)
		if err != nil {
			respCh <- connResp{err: err}

			return
		}

		client := ssh.NewClient(conn, chans, reqs)

		respCh <- connResp{nil, conn, client}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp := <-respCh:
			if resp.err != nil {
				return resp.err
			}
			c.conn = resp.conn
			c.Client = resp.client

			return nil
		}
	}
}

// SessionTarget selects where a session runs on the remote machine.
type SessionTarget struct {
	// Container names the container to run in. When empty, the machine decides
	// where the session lands.
	Container string

	// Machine runs the session in the machine's own namespace instead of in a
	// container. It is mutually exclusive with Container.
	Machine bool
}

func (t SessionTarget) validate() error {
	if t.Machine && t.Container != "" {
		return errors.New("session cannot target both a container and the machine")
	}

	return nil
}

func (c *Client) Shell(ctx context.Context, sessIO *SessionIO, cmd string, target SessionTarget) error {
	if err := target.validate(); err != nil {
		return err
	}

	if c.Client == nil {
		if err := c.Connect(ctx); err != nil {
			return err
		}
	}

	sess, err := c.Client.NewSession()

	if err != nil {
		return err
	}

	defer sess.Close()

	if err := setTarget(sess, target); err != nil {
		return err
	}

	return sessIO.attach(ctx, sess, cmd)
}

// setTarget tells the server where the session should run, which it reads as
// env on the session. Every kind of session says it the same way, so this is
// the one place that names the variables.
func setTarget(sess *ssh.Session, target SessionTarget) error {
	switch {
	case target.Machine:
		return sess.Setenv("FLY_SSH_MACHINE", "1")
	case target.Container != "":
		return sess.Setenv("FLY_SSH_CONTAINER", target.Container)
	}

	return nil
}

// SFTP opens an SFTP client against the session target: a container's
// filesystem, or the machine's own. The target has to be set on the session
// carrying the subsystem, and sftp.NewClient opens a session of its own with
// nothing set on it -- so the session is built here, where the target already
// gets set for shells.
//
// Closing the returned client closes that session with it.
func (c *Client) SFTP(ctx context.Context, target SessionTarget, opts ...sftp.ClientOption) (*sftp.Client, error) {
	if err := target.validate(); err != nil {
		return nil, err
	}

	if c.Client == nil {
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
	}

	sess, err := c.Client.NewSession()
	if err != nil {
		return nil, err
	}

	if err := setTarget(sess, target); err != nil {
		sess.Close()

		return nil, err
	}

	// Whatever the server has to say about a subsystem that never starts, it
	// says on stderr; the SFTP client would only report the silence that
	// follows.
	var stderr sessionStderr
	sess.Stderr = &stderr

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()

		return nil, err
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()

		return nil, err
	}

	if err := sess.RequestSubsystem("sftp"); err != nil {
		sess.Close()

		return nil, fmt.Errorf("request sftp subsystem: %w%s", err, stderr.reason())
	}

	client, err := sftp.NewClientPipe(stdout, sessionWriteCloser{stdin, sess}, opts...)
	if err != nil {
		sess.Close()

		return nil, fmt.Errorf("start sftp: %w%s", err, stderr.reason())
	}

	return client, nil
}

// sessionWriteCloser ties the session's life to the SFTP client's: pkg/sftp
// closes the stream it writes to when the client is closed, and the session
// that stream runs over has nothing left to do at that point.
type sessionWriteCloser struct {
	io.WriteCloser

	sess *ssh.Session
}

func (w sessionWriteCloser) Close() error {
	err := w.WriteCloser.Close()

	// A session the server has already finished with closes as EOF, which is
	// the ordinary end of a transfer rather than a failure to report.
	if cerr := w.sess.Close(); err == nil && !errors.Is(cerr, io.EOF) {
		err = cerr
	}

	return err
}

// sessionStderr collects a session's stderr, which the SSH library writes from
// its own goroutine while this one reads it.
type sessionStderr struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *sessionStderr) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

// reason renders what the server said, ready to append to an error message.
func (s *sessionStderr) reason() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if said := strings.TrimSpace(s.buf.String()); said != "" {
		return ": " + said
	}

	return ""
}
