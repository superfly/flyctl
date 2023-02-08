package ssh

// TODO - This file was copy and pasted from the cmd package and still needs to be
// updated to take on new conventions.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/ssh"
	"github.com/superfly/flyctl/terminal"
)

func spin(in, out string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	if !helpers.IsTerminal() {
		fmt.Fprintln(os.Stderr, in)
		return cancel
	}

	go func() {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Writer = os.Stderr
		s.Prefix = in
		s.FinalMSG = out
		s.Start()
		defer s.Stop()

		<-ctx.Done()
	}()

	return cancel
}

const DefaultSshUsername = "root"

type SSHParams struct {
	Ctx            context.Context
	Org            api.OrganizationImpl
	App            string
	Username       string
	Dialer         agent.Dialer
	Cmd            string
	Stdin          io.Reader
	Stdout         io.WriteCloser
	Stderr         io.WriteCloser
	DisableSpinner bool
}

func RunSSHCommand(ctx context.Context, app *api.AppCompact, dialer agent.Dialer, addr string, cmd string, username string) ([]byte, error) {
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
		return nil, fmt.Errorf(errBuf.String())
	}

	return outBuf.Bytes(), nil
}

func SSHConnect(p *SSHParams, addr string) error {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, pk, err := singleUseSSHCertificate(p.Ctx, p.Org)
	if err != nil {
		return fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh establish`)", err)
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

	terminal.Debugf("Connection completed.\n", addr)

	if !p.DisableSpinner {
		endSpin()
	}

	term := &ssh.Terminal{
		Stdin:  p.Stdin,
		Stdout: p.Stdout,
		Stderr: p.Stderr,
		Mode:   "xterm",
	}

	if err := sshClient.Shell(context.Background(), term, p.Cmd); err != nil {
		return errors.Wrap(err, "ssh shell")
	}

	return nil
}

func singleUseSSHCertificate(ctx context.Context, org api.OrganizationImpl) (*api.IssuedCertificate, ed25519.PrivateKey, error) {
	client := client.FromContext(ctx).API()
	hours := 1

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, err
	}

	icert, err := client.IssueSSHCertificate(ctx, org, []string{DefaultSshUsername, "fly"}, nil, &hours, pub)
	if err != nil {
		return nil, nil, err
	}

	return icert, priv, nil
}
