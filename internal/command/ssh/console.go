package ssh

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

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
	"github.com/superfly/flyctl/internal/appconfig"
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
		flag.Region(),
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
		flag.Bool{
			Name:        "pty",
			Description: "Allocate a pseudo-terminal (default: on when no command is provided)",
		},
		flag.String{
			Name:        "user",
			Shorthand:   "u",
			Description: "Unix username to connect as",
			Default:     DefaultSshUsername,
		},
		flag.ProcessGroup(),
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

	cmd := command.New(usage, short, long, runConsole, command.RequireSession, command.RequireAppName)

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
		sentry.WithContexts(map[string]sentry.Context{
			"app": map[string]interface{}{
				"name": app.Name,
			},
			"organization": map[string]interface{}{
				"name": app.Organization.Slug,
			},
		}),
	)
}

func runConsole(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	if !quiet(ctx) {
		terminal.Debugf("Retrieving app info for %s\n", appName)
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, dialer, err := BringUpAgent(ctx, client, app, quiet(ctx))
	if err != nil {
		return err
	}

	addr, err := lookupAddress(ctx, agentclient, dialer, app, true)
	if err != nil {
		return err
	}

	// TODO: eventually remove the exception for sh and bash.
	cmd := flag.GetString(ctx, "command")
	allocPTY := cmd == "" || flag.GetBool(ctx, "pty")
	if !allocPTY && (cmd == "sh" || cmd == "/bin/sh" || cmd == "bash" || cmd == "/bin/bash") {
		terminal.Warn(
			"Allocating a pseudo-terminal since the command provided is a shell. " +
				"This behavior will change in the future; please use --pty explicitly if this is what you want.",
		)
		allocPTY = true
	}

	params := &ConnectParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		Username:       flag.GetString(ctx, "user"),
		DisableSpinner: quiet(ctx),
	}
	sshc, err := Connect(params, addr)
	if err != nil {
		captureError(err, app)
		return err
	}

	if err := Console(ctx, sshc, cmd, allocPTY); err != nil {
		captureError(err, app)
		return err
	}

	return nil
}

func Console(ctx context.Context, sshClient *ssh.Client, cmd string, allocPTY bool) error {
	currentStdin, currentStdout, currentStderr, err := setupConsole()
	defer func() error {
		if err := cleanupConsole(currentStdin, currentStdout, currentStderr); err != nil {
			return err
		}
		return nil
	}()

	sessIO := &ssh.SessionIO{
		Stdin: os.Stdin,
		// "colorable" package should be used after the console setup performed above.
		// Otherwise, virtual terminal emulation provided by the package will break UTF-8 encoding.
		// If flyctl targets Windows 10+ only then we can avoid using this package at all
		// because Windows 10+ already provides virtual terminal support.
		Stdout:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStdout(), func() error { return nil }),
		Stderr:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStderr(), func() error { return nil }),
		AllocPTY: allocPTY,
		TermEnv:  determineTermEnv(),
	}

	if err := sshClient.Shell(ctx, sessIO, cmd); err != nil {
		return errors.Wrap(err, "ssh shell")
	}

	return err
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
		return "", fmt.Errorf("app %s has no started VMs.\nIt may be unhealthy or not have been deployed yet.\nTry the following command to verify:\n\nfly status", app.Name)
	}

	if region := flag.GetRegion(ctx); region != "" {
		machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
			return m.Region == region
		})
		if len(machines) < 1 {
			return "", fmt.Errorf("app %s has no VMs in region %s", app.Name, region)
		}
	}

	if group := flag.GetProcessGroup(ctx); group != "" {
		machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
			return m.ProcessGroup() == group
		})
		if len(machines) < 1 {
			return "", fmt.Errorf("app %s has no VMs in process group %s", app.Name, group)
		}
	}

	var namesWithRegion []string
	var selectedMachine *api.Machine
	multipleGroups := len(lo.UniqBy(machines, func(m *api.Machine) string { return m.ProcessGroup() })) > 1

	for _, machine := range machines {
		nameWithRegion := fmt.Sprintf("%s: %s %s %s", machine.Region, machine.ID, machine.PrivateIP, machine.Name)

		role := ""
		for _, check := range machine.Checks {
			if check.Name == "role" {
				if check.Status == api.Passing {
					role = check.Output
				} else {
					role = "error"
				}
			}
		}

		if role != "" {
			nameWithRegion += fmt.Sprintf(" (%s)", role)
		}

		if multipleGroups {
			nameWithRegion += fmt.Sprintf(" (%s)", machine.ProcessGroup())
		}
		namesWithRegion = append(namesWithRegion, nameWithRegion)
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
			_, err := flapsClient.Start(ctx, selectedMachine.ID, "")
			if err != nil {
				return "", err
			}

			err = flapsClient.Wait(ctx, selectedMachine, "started", 60*time.Second)

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

const defaultTermEnv = "xterm"

func determineTermEnv() string {
	switch runtime.GOOS {
	case "aix":
		return determineTermEnvFromLocalEnv()
	case "darwin":
		return determineTermEnvFromLocalEnv()
	case "dragonfly":
		return determineTermEnvFromLocalEnv()
	case "freebsd":
		return determineTermEnvFromLocalEnv()
	case "illumos":
		return determineTermEnvFromLocalEnv()
	case "linux":
		return determineTermEnvFromLocalEnv()
	case "netbsd":
		return determineTermEnvFromLocalEnv()
	case "openbsd":
		return determineTermEnvFromLocalEnv()
	case "solaris":
		return determineTermEnvFromLocalEnv()
	default:
		return defaultTermEnv
	}
}

func determineTermEnvFromLocalEnv() string {
	if term := os.Getenv("TERM"); term != "" {
		return term
	} else {
		return defaultTermEnv
	}
}
