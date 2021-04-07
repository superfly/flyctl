package ssh

import (
	"context"
	"errors"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	Addr string
	User string

	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	PrivateKey, Certificate string

	client *ssh.Client
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
		return err
	}

	conf := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
	}

	conn, chans, reqs, err := ssh.NewClientConn(tcpConn, tcpConn.RemoteAddr().String(), conf)
	if err != nil {
		return err
	}

	c.conn, c.client = conn, ssh.NewClient(conn, chans, reqs)
	return nil
}

func (c *Client) Shell(ctx context.Context, term *Terminal) error {
	if c.client == nil {
		if err := c.Connect(ctx); err != nil {
			return err
		}
	}

	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	return term.attach(ctx, sess)
}
