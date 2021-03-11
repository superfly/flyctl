package docker

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
)

var unauthorizedError = errors.New("You are unauthorized to use this builder")

func isUnauthorized(err error) bool {
	return errors.Is(err, unauthorizedError)
}

func isRetyableError(err error) bool {
	if isUnauthorized(err) {
		return false
	}
	return true
}

func WaitForDaemon(ctx context.Context, client *dockerclient.Client) error {
	deadline := time.After(5 * time.Minute)

	b := &backoff.Backoff{
		//These are the defaults
		Min:    200 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 1.2,
		Jitter: true,
	}

	consecutiveSuccesses := 0
	var healthyStart time.Time

OUTER:
	for {
		checkErr := make(chan error, 1)

		go func() {
			_, err := client.Ping(ctx)
			checkErr <- err
		}()

		select {
		case err := <-checkErr:
			if err == nil {
				if consecutiveSuccesses == 0 {
					// reset on the first success in a row so the next checks are a bit spaced out
					healthyStart = time.Now()
					b.Reset()
				}
				consecutiveSuccesses++

				if time.Since(healthyStart) > 3*time.Second {
					terminal.Info("Remote builder is ready to build!")
					break OUTER
				}

				dur := b.Duration()
				terminal.Debugf("Remote builder available, but pinging again in %s to be sure\n", dur)
				time.Sleep(dur)
			} else {
				if !isRetyableError(err) {
					return err
				}
				consecutiveSuccesses = 0
				dur := b.Duration()
				terminal.Debugf("Remote builder unavailable, retrying in %s (err: %v)\n", dur, err)
				time.Sleep(dur)
			}
		case <-deadline:
			return fmt.Errorf("Could not ping remote builder within 5 minutes, aborting.")
		case <-ctx.Done():
			terminal.Warn("Canceled")
			break OUTER
		}
	}

	return nil
}

type remoteBuilderConnection struct {
	client  *ssh.Client
	session *ssh.Session

	r io.Reader
	w io.WriteCloser
}

func (c *remoteBuilderConnection) Close() error {
	err1 := c.session.Close()
	err2 := c.client.Close()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

func (c *remoteBuilderConnection) Read(b []byte) (n int, err error) {
	return c.r.Read(b)
}

func (c *remoteBuilderConnection) Write(b []byte) (n int, err error) {
	return c.w.Write(b)
}

func (c *remoteBuilderConnection) LocalAddr() net.Addr {
	return c.client.LocalAddr()
}

func (c *remoteBuilderConnection) RemoteAddr() net.Addr {
	return c.client.RemoteAddr()
}

func (c *remoteBuilderConnection) SetDeadline(t time.Time) error {
	return nil
}

func (c *remoteBuilderConnection) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *remoteBuilderConnection) SetWriteDeadline(t time.Time) error {
	return nil
}

func newRemoteBuilderConnection(host string, port int, appName string, authToken string) (net.Conn, error) {
	config := &ssh.ClientConfig{
		User: appName,
		Auth: []ssh.AuthMethod{
			ssh.Password(authToken),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	var failed bool

	client, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)), config)
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, unauthorizedError
		}
		return nil, errors.Wrap(err, "Error creating ssh client")
	}
	defer func() {
		if failed {
			client.Close()
		}
	}()

	session, err := client.NewSession()
	if err != nil {
		return nil, errors.Wrap(err, "Error creating ssh session")
	}
	defer func() {
		if failed {
			session.Close()
		}
	}()

	sessStderr, err := session.StderrPipe()
	if err != nil {
		failed = true
		return nil, errors.Wrap(err, "Error creating srderr pipe")
	}
	go io.Copy(os.Stderr, sessStderr)

	sessStdOut, err := session.StdoutPipe()
	if err != nil {
		failed = true
		return nil, errors.Wrap(err, "Error creating stdout pipe")
	}

	sessStdin, err := session.StdinPipe()
	if err != nil {
		failed = true
		return nil, errors.Wrap(err, "Error creating stdin pipe")
	}

	err = session.Start("start")
	if err != nil {
		failed = true
		return nil, errors.Wrap(err, "Error starting session")
	}

	conn := &remoteBuilderConnection{
		client:  client,
		session: session,
		w:       sessStdin,
		r:       sessStdOut,
	}

	return conn, nil
}
