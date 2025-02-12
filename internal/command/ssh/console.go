package ssh

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/mattn/go-colorable"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
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
		flag.String{
			Name:        "machine",
			Default:     "",
			Description: "Run the console in the existing machine with the specified ID",
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
		flag.String{
			Name:        "container",
			Description: "Container to connect to",
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
		flag.ProcessGroup(""),
	)
}

func quiet(ctx context.Context) bool {
	return flag.GetBool(ctx, "quiet")
}

func lookupAddress(ctx context.Context, cli *agent.Client, dialer agent.Dialer, app *fly.AppCompact, console bool) (addr string, err error) {
	addr, err = addrForMachines(ctx, app, console)
	if err != nil {
		return
	}

	// wait for the addr to be resolved in dns unless it's an ip address
	if !ip.IsV6(addr) {
		if err := cli.WaitForDNS(ctx, dialer, app.Organization.Slug, addr, ""); err != nil {
			captureError(ctx, err, app)
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

func captureError(ctx context.Context, err error, app *fly.AppCompact) {
	// ignore cancelled errors
	if errors.Is(err, context.Canceled) {
		return
	}

	sentry.CaptureException(err,
		sentry.WithTraceID(ctx),
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
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	if !quiet(ctx) {
		terminal.Debugf("Retrieving app info for %s\n", appName)
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	network, err := client.GetAppNetwork(ctx, app.Name)
	if err != nil {
		return fmt.Errorf("get app network: %w", err)
	}

	agentclient, dialer, err := BringUpAgent(ctx, client, app, *network, quiet(ctx))
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
		Container:      flag.GetString(ctx, "container"),
		AppNames:       []string{app.Name},
	}
	sshc, err := Connect(params, addr)
	if err != nil {
		captureError(ctx, err, app)
		return err
	}

	if err := Console(ctx, sshc, cmd, allocPTY, params.Container); err != nil {
		captureError(ctx, err, app)
		return err
	}

	return nil
}

func Console(ctx context.Context, sshClient *ssh.Client, cmd string, allocPTY bool, container string) error {
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

	if err := sshClient.Shell(ctx, sessIO, cmd, container); err != nil {
		return errors.Wrap(err, "ssh shell")
	}

	return err
}

func addrForMachines(ctx context.Context, app *fly.AppCompact, console bool) (addr string, err error) {
	out := iostreams.FromContext(ctx).Out
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return "", err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return "", err
	}

	machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
		return m.State == "started"
	})

	if len(machines) < 1 {
		return "", fmt.Errorf("app %s has no started VMs.\nIt may be unhealthy or not have been deployed yet.\nTry the following command to verify:\n\nfly status", app.Name)
	}

	if region := flag.GetRegion(ctx); region != "" {
		machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
			return m.Region == region
		})
		if len(machines) < 1 {
			return "", fmt.Errorf("app %s has no VMs in region %s", app.Name, region)
		}
	}

	if group := flag.GetProcessGroup(ctx); group != "" {
		machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
			return m.ProcessGroup() == group
		})
		if len(machines) < 1 {
			return "", fmt.Errorf("app %s has no VMs in process group %s", app.Name, group)
		}
	}

	var namesWithRegion []string
	machineID := flag.GetString(ctx, "machine")
	var selectedMachine *fly.Machine
	multipleGroups := len(lo.UniqBy(machines, func(m *fly.Machine) string { return m.ProcessGroup() })) > 1

	for _, machine := range machines {
		if machine.ID == machineID {
			selectedMachine = machine
		}

		nameWithRegion := fmt.Sprintf("%s: %s %s %s", machine.Region, machine.ID, machine.PrivateIP, machine.Name)

		role := ""
		for _, check := range machine.Checks {
			if check.Name == "role" {
				if check.Status == fly.Passing {
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
		if flag.IsSpecified(ctx, "machine") {
			return "", errors.New("--machine can't be used with -s/--select")
		}

		selected := 0

		if prompt.Select(ctx, &selected, "Select VM:", "", namesWithRegion...); err != nil {
			return "", fmt.Errorf("selecting VM: %w", err)
		}

		selectedMachine = machines[selected]
	}

	if selectedMachine != nil {
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
