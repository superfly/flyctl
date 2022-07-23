package ssh

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/ip"
	"github.com/superfly/flyctl/ssh"
	"github.com/superfly/flyctl/terminal"
)

func newConsole() *cobra.Command {
	const (
		long  = `Connect to a running instance of the current app.`
		short = long
		usage = "console"
	)

	cmd := command.New(usage, short, long, runConsole, command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.Org(),
		flag.App(),
		flag.String{
			Name:        "command",
			Shorthand:   "C",
			Default:     "",
			Description: "command to run on SSH session",
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Default:     false,
			Description: "select available instances",
		},
		flag.String{
			Name:        "region",
			Shorthand:   "r",
			Description: "Region to create WireGuard connection in",
		},
	)

	return cmd
}

func runConsole(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)

	terminal.Debugf("Retrieving app info for %s\n", appName)

	app, err := client.GetAppBasic(ctx, appName)

	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	captureError := func(err error) {
		// ignore cancelled errors
		if errors.Is(err, context.Canceled) {
			return
		}

		sentry.CaptureException(err,
			sentry.WithTag("feature", "ssh-console"),
			sentry.WithContexts(map[string]interface{}{
				"app": map[string]interface{}{
					"name": app.Name,
				},
				"organization": map[string]interface{}{
					"name": app.Organization.Slug,
				},
			}),
		)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		captureError(err)
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		captureError(err)
		return fmt.Errorf("ssh: can't build tunnel for %s: %s\n", app.Organization.Slug, err)
	}

	io.StartProgressIndicatorMsg("Connecting to tunnel")
	if err := agentclient.WaitForTunnel(ctx, app.Organization.Slug); err != nil {
		captureError(err)
		return errors.Wrapf(err, "tunnel unavailable")
	}
	io.StopProgressIndicator()

	var addr string

	if flag.GetBool(ctx, "select") {
		instances, err := agentclient.Instances(ctx, app.Organization.Slug, appName)
		if err != nil {
			return fmt.Errorf("look up %s: %w", appName, err)
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
	} else if len(flag.Args(ctx)) != 0 {
		addr = flag.Args(ctx)[0]
	} else {
		addr = fmt.Sprintf("top1.nearest.of.%s.internal", appName)
	}

	// wait for the addr to be resolved in dns unless it's an ip address
	if !ip.IsV6(addr) {
		if err := agentclient.WaitForDNS(ctx, dialer, app.Organization.Slug, addr); err != nil {
			captureError(err)
			return errors.Wrapf(err, "host unavailable")
		}
	}

	err = sshConnect(&SSHParams{
		Ctx:    ctx,
		Org:    app.Organization,
		Dialer: dialer,
		App:    appName,
		Cmd:    flag.GetString(ctx, "command"),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, addr)

	if err != nil {
		captureError(err)
	}

	return err
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

	pemkey := marshalED25519PrivateKey(pk, "single-use certificate")

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

	if err := sshClient.Connect(p.Ctx); err != nil {
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

	if err := sshClient.Shell(p.Ctx, term, p.Cmd); err != nil {
		return errors.Wrap(err, "ssh shell")
	}

	return nil
}
