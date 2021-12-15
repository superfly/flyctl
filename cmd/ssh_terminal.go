package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/ssh"
	"github.com/superfly/flyctl/terminal"
)

func runSSHCommand(cmdCtx *cmdctx.CmdContext, app *api.App, dialer agent.Dialer, cmd string) ([]byte, error) {
	var inBuf bytes.Buffer
	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	stdoutWriter := ioutils.NewWriteCloserWrapper(&outBuf, func() error { return nil })
	stderrWriter := ioutils.NewWriteCloserWrapper(&errBuf, func() error { return nil })
	inReader := ioutils.NewReadCloserWrapper(&inBuf, func() error { return nil })

	addr := fmt.Sprintf("%s.internal", app.Name)

	err := sshConnect(&SSHParams{
		Ctx:            cmdCtx,
		Org:            &app.Organization,
		Dialer:         dialer,
		App:            app.Name,
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

func runSSHConsole(cc *cmdctx.CmdContext) error {
	client := cc.Client.API()
	ctx := cc.Command.Context()

	terminal.Debugf("Retrieving app info for %s\n", cc.AppName)

	app, err := client.GetApp(ctx, cc.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	captureError := func(err error) {
		// ignore cancelled errors
		if errors.Is(err, context.Canceled) {
			return
		}

		flyerr.CaptureException(err,
			flyerr.WithTag("feature", "ssh-console"),
			flyerr.WithContexts(map[string]interface{}{
				"app":          app.Name,
				"organization": app.Organization.Slug,
			}),
		)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		captureError(err)
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, &app.Organization)
	if err != nil {
		captureError(err)
		return fmt.Errorf("ssh: can't build tunnel for %s: %s\n", app.Organization.Slug, err)
	}

	cc.IO.StartProgressIndicatorMsg("Connecting to tunnel")
	if err := agentclient.WaitForTunnel(ctx, &app.Organization); err != nil {
		captureError(err)
		return errors.Wrapf(err, "tunnel unavailable")
	}
	cc.IO.StopProgressIndicator()

	var addr string

	if cc.Config.GetBool("select") {
		instances, err := agentclient.Instances(ctx, &app.Organization, cc.AppName)
		if err != nil {
			return fmt.Errorf("look up %s: %w", cc.AppName, err)
		}

		selected := 0
		prompt := &survey.Select{
			Message:  "Select instance:",
			Options:  instances.Labels,
			PageSize: 15,
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return fmt.Errorf("selecting instance: %w", err)
		}

		addr = fmt.Sprintf("[%s]", instances.Addresses[selected])
	} else if len(cc.Args) != 0 {
		addr = cc.Args[0]
	} else {
		addr = fmt.Sprintf("top1.nearest.of.%s.internal", cc.AppName)
	}

	// wait for the addr to be resolved in dns unless it's an ip address
	if !agent.IsIPv6(addr) {
		cc.IO.StartProgressIndicatorMsg("Waiting for host")
		if err := agentclient.WaitForHost(ctx, &app.Organization, addr); err != nil {
			captureError(err)
			return errors.Wrapf(err, "host unavailable")
		}
		cc.IO.StopProgressIndicator()
	}

	err = sshConnect(&SSHParams{
		Ctx:    cc,
		Org:    &app.Organization,
		Dialer: dialer,
		App:    cc.AppName,
		Cmd:    cc.Config.GetString("command"),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, addr)

	if err != nil {
		captureError(err)
	}

	return err
}

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

type SSHParams struct {
	Ctx            *cmdctx.CmdContext
	Org            *api.Organization
	App            string
	Dialer         agent.Dialer
	Cmd            string
	Stdin          io.Reader
	Stdout         io.WriteCloser
	Stderr         io.WriteCloser
	DisableSpinner bool
}

func sshConnect(p *SSHParams, addr string) error {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, err := singleUseSSHCertificate(p.Ctx, p.Org)
	if err != nil {
		return fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh establish`)", err)
	}

	pk, err := parsePrivateKey(cert.Key)
	if err != nil {
		return errors.Wrap(err, "parse ssh certificate")
	}

	pemkey := MarshalED25519PrivateKey(pk, "single-use certificate")

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &ssh.Client{
		Addr: addr + ":22",
		User: "root",

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
