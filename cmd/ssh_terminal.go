package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/ssh"
	"github.com/superfly/flyctl/terminal"
)

func runSSHConsole(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	terminal.Debugf("Retrieving app info for %s\n", ctx.AppName)

	app, err := client.GetApp(ctx.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, err := EstablishFlyAgent(ctx)
	if err != nil {
		return fmt.Errorf("can't establish agent: %s\n", err)
		return err
	}

	dialer, err := agentclient.Dialer(&app.Organization)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s\n", app.Organization.Slug, err)
	}

	if ctx.Config.GetBool("probe") {
		if err = agentclient.Probe(&app.Organization); err != nil {
			return fmt.Errorf("probe wireguard: %w", err)
		}
	}

	var addr string

	if ctx.Config.GetBool("select") {
		instances, err := agentclient.Instances(&app.Organization, ctx.AppName)
		if err != nil {
			return fmt.Errorf("look up %s: %w", ctx.AppName, err)
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
	} else if len(ctx.Args) != 0 {
		addr = ctx.Args[0]
	} else {
		addr = fmt.Sprintf("%s.internal", ctx.AppName)
	}

	return sshConnect(&SSHParams{
		Ctx:    ctx,
		Org:    &app.Organization,
		Dialer: dialer,
		App:    ctx.AppName,
		Cmd:    ctx.Config.GetString("command"),
	}, addr)
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
		return
	}()

	return cancel
}

type SSHParams struct {
	Ctx    *cmdctx.CmdContext
	Org    *api.Organization
	App    string
	Dialer *agent.Dialer
	Cmd    string
}

func sshConnect(p *SSHParams, addr string) error {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, err := singleUseSSHCertificate(p.Ctx, p.Org)
	if err != nil {
		return fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh establish`)", err)
	}

	pk, err := parsePrivateKey(cert.Key)
	if err != nil {
		return fmt.Errorf("parse ssh certificate: %w", err)
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

	endSpin := spin(fmt.Sprintf("Connecting to %s...", addr),
		fmt.Sprintf("Connecting to %s... complete\n", addr))
	defer endSpin()

	if err := sshClient.Connect(context.Background()); err != nil {
		return fmt.Errorf("connect to SSH server: %w", err)
	}
	defer sshClient.Close()

	terminal.Debugf("Connection completed.\n", addr)

	endSpin()

	term := &ssh.Terminal{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Mode:   "xterm",
	}

	if err := sshClient.Shell(context.Background(), term, p.Cmd); err != nil {
		return fmt.Errorf("SSH shell: %w", err)
	}

	return nil
}
