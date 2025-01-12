package ssh

// TODO - This file was copy and pasted from the cmd package and still needs to be
// updated to take on new conventions.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/pkg/errors"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/ssh"
	"github.com/superfly/flyctl/terminal"
)

type SSHParams struct {
	Ctx            context.Context
	Org            fly.OrganizationImpl
	App            string
	Username       string
	Dialer         agent.Dialer
	Cmd            string
	Stdin          io.Reader
	Stdout         io.WriteCloser
	Stderr         io.WriteCloser
	DisableSpinner bool
}

func RunSSHCommand(ctx context.Context, app *fly.AppCompact, dialer agent.Dialer, addr string, cmd string, username string) ([]byte, error) {
	var inBuf bytes.Buffer
	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	stdoutWriter := ioutils.NewWriteCloserWrapper(&outBuf, func() error { return nil })
	stderrWriter := ioutils.NewWriteCloserWrapper(&errBuf, func() error { return nil })
	inReader := ioutils.NewReadCloserWrapper(&inBuf, func() error { return nil })

	err := SSHConnect(&SSHParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		App:            app.Name,
		Username:       username,
		Cmd:            cmd,
		Stdin:          inReader,
		Stdout:         stdoutWriter,
		Stderr:         stderrWriter,
		DisableSpinner: true,
	}, addr)
	if err != nil {
		return nil, err
	}

	if len(errBuf.Bytes()) > 0 {
		return nil, errors.New(errBuf.String())
	}

	return outBuf.Bytes(), nil
}

func SSHConnect(p *SSHParams, addr string) error {
	terminal.Debugf("Fetching certificate for %s at %s\n", p.App, addr)

	var appNames []string
	if p.App != "" {
		appNames = append(appNames, p.App)
	}

	cert, pk, err := singleUseSSHCertificate(p.Ctx, p.Org, appNames, p.Username)
	if err != nil {
		return fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh issue`)", err)
	}

	pemkey := ssh.MarshalED25519PrivateKey(pk, "single-use certificate")

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &ssh.Client{
		Addr: net.JoinHostPort(addr, "22"),
		User: p.Username,

		Dial: p.Dialer.DialContext,

		Certificate: cert.Certificate,
		PrivateKey:  string(pemkey),
	}

	var endSpin context.CancelFunc
	if !p.DisableSpinner {
		endSpin = spin(fmt.Sprintf("Connecting to %s...", addr),
			fmt.Sprintf("Connecting to %s... complete\n", addr))
		defer endSpin()
	}

	if err := sshClient.Connect(context.Background()); err != nil {
		return errors.Wrap(err, "error connecting to SSH server")
	}
	defer sshClient.Close()

	terminal.Debugf("Connection %s completed.\n", addr)

	if !p.DisableSpinner {
		endSpin()
	}

	sessIO := &ssh.SessionIO{
		Stdin:    p.Stdin,
		Stdout:   p.Stdout,
		Stderr:   p.Stderr,
		AllocPTY: true,
		TermEnv:  "xterm",
	}

	if err := sshClient.Shell(context.Background(), sessIO, p.Cmd); err != nil {
		return errors.Wrap(err, "ssh shell")
	}

	return nil
}
