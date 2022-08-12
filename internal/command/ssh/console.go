package ssh

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
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
		flag.AppConfig(),
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

	app, err := client.GetAppCompact(ctx, appName)
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

	if app.PlatformVersion == "machines" {
		addr, err = addrForMachines(ctx, app)
	} else {
		addr, err = addrForNomad(ctx, agentclient, app)
	}

	if err != nil {
		return err
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

func addrForMachines(ctx context.Context, app *api.AppCompact) (addr string, err error) {
	out := iostreams.FromContext(ctx).Out
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return "", err
	}

	machines, err := flapsClient.List(ctx, "")

	if len(machines) < 1 {
		return "", fmt.Errorf("app %s has no VMs", app.Name)
	}

	if err != nil {
		return "", err
	}

	var namesWithRegion []string
	var selectedMachine *api.Machine

	for _, machine := range machines {
		namesWithRegion = append(namesWithRegion, fmt.Sprintf("%s: %s %s %s", machine.Region, machine.ID, machine.PrivateIP, machine.Name))
	}

	if flag.GetBool(ctx, "select") {

		selected := 0

		prompt := &survey.Select{
			Message:  "Select VM:",
			Options:  namesWithRegion,
			PageSize: 15,
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return "", fmt.Errorf("selecting VM: %w", err)
		}

		selectedMachine = machines[selected]

		if selectedMachine.State != "started" {
			fmt.Fprintf(out, "Starting machine %s..", selectedMachine.ID)
			_, err := flapsClient.Start(ctx, selectedMachine.ID)
			if err != nil {
				return "", err
			}

			err = flapsClient.Wait(ctx, selectedMachine, "started")

			if err != nil {
				return "", err
			}

		}
	}

	if len(flag.Args(ctx)) != 0 {
		return flag.Args(ctx)[0], nil
	}

	if selectedMachine == nil {
		selectedMachine = machines[0]
	}
	// No VM was selected or passed as an argument, so just pick the first one for now
	// Later, we might want to use 'nearest.of' but also resolve the machine IP to be able to start it
	return fmt.Sprintf("[%s]", selectedMachine.PrivateIP), nil
}

func addrForNomad(ctx context.Context, agentclient *agent.Client, app *api.AppCompact) (addr string, err error) {
	if flag.GetBool(ctx, "select") {

		instances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
		if err != nil {
			return "", fmt.Errorf("look up %s: %w", app.Name, err)
		}

		selected := 0
		prompt := &survey.Select{
			Message:  "Select instance:",
			Options:  instances.Labels,
			PageSize: 15,
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return "", fmt.Errorf("selecting instance: %w", err)
		}

		addr = fmt.Sprintf("[%s]", instances.Addresses[selected])
		return addr, nil
	}

	if len(flag.Args(ctx)) != 0 {
		return flag.Args(ctx)[0], nil
	}

	return fmt.Sprintf("top1.nearest.of.%s.internal", app.Name), nil
}
