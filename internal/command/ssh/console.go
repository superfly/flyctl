package ssh

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/mattn/go-colorable"
	"github.com/pkg/errors"
	"github.com/samber/lo"
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

func stdArgsSSH(cmd *cobra.Command) {
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
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Don't print progress indicators for WireGuard",
		},
		flag.String{
			Name:        "address",
			Shorthand:   "A",
			Description: "Address of VM to connect to",
		},
	)
}

func quiet(ctx context.Context) bool {
	return flag.GetBool(ctx, "quiet")
}

func lookupAddress(ctx context.Context, cli *agent.Client, dialer agent.Dialer, app *api.AppCompact, console bool) (addr string, err error) {
	if app.PlatformVersion == "machines" {
		addr, err = addrForMachines(ctx, app, console)
	} else {
		addr, err = addrForNomad(ctx, cli, app, console)
	}

	if err != nil {
		return
	}

	// wait for the addr to be resolved in dns unless it's an ip address
	if !ip.IsV6(addr) {
		if err := cli.WaitForDNS(ctx, dialer, app.Organization.Slug, addr); err != nil {
			captureError(err, app)
			return "", errors.Wrapf(err, "host unavailable at %s", addr)
		}
	}

	return
}

func newConsole() *cobra.Command {
	const (
		long  = `Connect to a running instance of the current app.`
		short = long
		usage = "console"
	)

	cmd := command.New(usage, short, long, runConsole, command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.MaximumNArgs(1)

	stdArgsSSH(cmd)

	return cmd
}

func captureError(err error, app *api.AppCompact) {
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

func bringUp(ctx context.Context, client *api.Client, app *api.AppCompact) (*agent.Client, agent.Dialer, error) {
	io := iostreams.FromContext(ctx)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		captureError(err, app)
		return nil, nil, errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		captureError(err, app)
		return nil, nil, fmt.Errorf("ssh: can't build tunnel for %s: %s\n", app.Organization.Slug, err)
	}

	if !quiet(ctx) {
		io.StartProgressIndicatorMsg("Connecting to tunnel")
	}
	if err := agentclient.WaitForTunnel(ctx, app.Organization.Slug); err != nil {
		captureError(err, app)
		return nil, nil, errors.Wrapf(err, "tunnel unavailable")
	}
	if !quiet(ctx) {
		io.StopProgressIndicator()
	}

	return agentclient, dialer, nil
}

func runConsole(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	if !quiet(ctx) {
		terminal.Debugf("Retrieving app info for %s\n", appName)
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, dialer, err := bringUp(ctx, client, app)
	if err != nil {
		return err
	}

	addr, err := lookupAddress(ctx, agentclient, dialer, app, true)
	if err != nil {
		return err
	}

	// BUG(tqbf): many of these are no longer really params
	params := &SSHParams{
		Ctx:    ctx,
		Org:    app.Organization,
		Dialer: dialer,
		App:    appName,
		Cmd:    flag.GetString(ctx, "command"),
		Stdin:  os.Stdin,
		Stdout: ioutils.NewWriteCloserWrapper(colorable.NewColorableStdout(), func() error { return nil }),
		Stderr: ioutils.NewWriteCloserWrapper(colorable.NewColorableStderr(), func() error { return nil }),
	}

	if quiet(ctx) {
		params.DisableSpinner = true
	}

	sshc, err := sshConnect(params, addr)
	if err != nil {
		captureError(err, app)
		return err
	}

	term := &ssh.Terminal{
		Stdin:  params.Stdin,
		Stdout: params.Stdout,
		Stderr: params.Stderr,
		Mode:   "xterm",
	}

	if err := sshc.Shell(params.Ctx, term, params.Cmd); err != nil {
		captureError(err, app)
		return errors.Wrap(err, "ssh shell")
	}

	return err
}

func sshConnect(p *SSHParams, addr string) (*ssh.Client, error) {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, pk, err := singleUseSSHCertificate(p.Ctx, p.Org)
	if err != nil {
		return nil, fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh establish`)", err)
	}

	pemkey := ssh.MarshalED25519PrivateKey(pk, "single-use certificate")

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &ssh.Client{
		Addr: net.JoinHostPort(addr, "22"),
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
		return nil, errors.Wrap(err, "error connecting to SSH server")
	}

	terminal.Debugf("Connection completed.\n", addr)

	if !p.DisableSpinner {
		endSpin()
	}

	return sshClient, nil
}

func addrForMachines(ctx context.Context, app *api.AppCompact, console bool) (addr string, err error) {
	out := iostreams.FromContext(ctx).Out
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return "", err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return "", err
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return m.State == "started"
	})

	if len(machines) < 1 {
		return "", fmt.Errorf("app %s has no started VMs", app.Name)
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

	if addr = flag.GetString(ctx, "address"); addr != "" {
		return addr, nil
	}

	if console {
		if len(flag.Args(ctx)) != 0 {
			return flag.Args(ctx)[0], nil
		}
	}

	if selectedMachine == nil {
		selectedMachine = machines[0]
	}
	// No VM was selected or passed as an argument, so just pick the first one for now
	// Later, we might want to use 'nearest.of' but also resolve the machine IP to be able to start it
	return selectedMachine.PrivateIP, nil
}

func addrForNomad(ctx context.Context, agentclient *agent.Client, app *api.AppCompact, console bool) (addr string, err error) {
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

		addr = instances.Addresses[selected]
		return addr, nil
	}

	if addr = flag.GetString(ctx, "address"); addr != "" {
		return addr, nil
	}

	if console {
		if len(flag.Args(ctx)) != 0 {
			return flag.Args(ctx)[0], nil
		}
	}

	// No VM was selected or passed as an argument, so just pick the first one for now
	// We may use 'nearest.of' in the future
	instances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
	if err != nil {
		return "", fmt.Errorf("look up %s: %w", app.Name, err)
	}
	if len(instances.Addresses) < 1 {
		return "", fmt.Errorf("no instances found for %s", app.Name)
	}
	return instances.Addresses[0], nil
}
