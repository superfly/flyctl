package console

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

func New() *cobra.Command {
	const (
		usage = "console"
		short = "Run a console in a new or existing machine"
		long  = "Run a console in a new or existing machine. The console command is\n" +
			"specified by the `console_command` configuration field. By default, a\n" +
			"new machine is created by default using the app's most recently deployed\n" +
			"image. An existing machine can be used instead with --machine."
	)
	cmd := command.New(usage, short, long, runConsole, command.RequireSession, command.RequireAppName)

	cmd.Args = cobra.NoArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.String{
			Name:        "machine",
			Description: "Run the console in the existing machine with the specified ID",
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Description: "Select the machine on which to execute the console from a list",
			Default:     false,
		},
		flag.String{
			Name:        "user",
			Shorthand:   "u",
			Description: "Unix username to connect as",
			Default:     ssh.DefaultSshUsername,
		},
		flag.VMSizeFlags,
	)

	return cmd
}

func runConsole(ctx context.Context) error {
	var (
		io        = iostreams.FromContext(ctx)
		appName   = appconfig.NameFromContext(ctx)
		apiClient = client.FromContext(ctx).API()
	)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app: %w", err)
	}

	if app.PlatformVersion != "machines" {
		return errors.New("console is only supported for the machines platform")
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig == nil {
		appConfig, err = appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to fetch app config from backend: %w", err)
		}
	}

	if err, extraInfo := appConfig.ValidateForMachinesPlatform(ctx); err != nil {
		fmt.Fprintln(io.ErrOut, extraInfo)
		return err
	}

	machine, cleanup, err := selectMachine(ctx, app, appConfig)
	if err != nil {
		return err
	}

	if cleanup != nil {
		defer cleanup()
	}

	_, dialer, err := ssh.BringUpAgent(ctx, apiClient, app, false)
	if err != nil {
		return err
	}

	params := &ssh.ConnectParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		Username:       flag.GetString(ctx, "user"),
		DisableSpinner: false,
	}
	sshClient, err := ssh.Connect(params, machine.PrivateIP)
	if err != nil {
		return err
	}

	return ssh.Console(ctx, sshClient, appConfig.ConsoleCommand, true)
}

func selectMachine(ctx context.Context, app *api.AppCompact, appConfig *appconfig.Config) (*api.Machine, func(), error) {
	if flag.GetBool(ctx, "select") {
		return promptForMachine(ctx, app, appConfig)
	} else if flag.IsSpecified(ctx, "machine") {
		return getMachineByID(ctx)
	} else {
		guest, err := determineEphemeralConsoleMachineGuest(ctx)
		if err != nil {
			return nil, nil, err
		}
		return makeEphemeralConsoleMachine(ctx, app, appConfig, guest)
	}
}

func promptForMachine(ctx context.Context, app *api.AppCompact, appConfig *appconfig.Config) (*api.Machine, func(), error) {
	if flag.IsSpecified(ctx, "machine") {
		return nil, nil, errors.New("--machine can't be used with -s/--select")
	}

	flapsClient := flaps.FromContext(ctx)
	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return nil, nil, err
	}
	machines = lo.Filter(machines, func(machine *api.Machine, _ int) bool {
		return machine.State == api.MachineStateStarted
	})

	ephemeralGuest, err := determineEphemeralConsoleMachineGuest(ctx)
	if err != nil {
		return nil, nil, err
	}
	cpuS := lo.Ternary(ephemeralGuest.CPUs == 1, "", "s")
	ephemeralGuestStr := fmt.Sprintf("%d %s CPU%s, %d MB of memory", ephemeralGuest.CPUs, ephemeralGuest.CPUKind, cpuS, ephemeralGuest.MemoryMB)

	options := []string{fmt.Sprintf("create an ephemeral machine (%s)", ephemeralGuestStr)}
	for _, machine := range machines {
		options = append(options, fmt.Sprintf("%s: %s %s %s", machine.Region, machine.ID, machine.PrivateIP, machine.Name))
	}

	index := 0
	if err := prompt.Select(ctx, &index, "Select a machine:", "", options...); err != nil {
		return nil, nil, fmt.Errorf("failed to prompt for a machine: %w", err)
	}
	if index == 0 {
		return makeEphemeralConsoleMachine(ctx, app, appConfig, ephemeralGuest)
	} else {
		return machines[index-1], nil, nil
	}
}

func getMachineByID(ctx context.Context) (*api.Machine, func(), error) {
	if flag.IsSpecified(ctx, "vm-cpus") {
		return nil, nil, errors.New("--vm-cpus can't be used with --machine")
	}
	if flag.IsSpecified(ctx, "vm-memory") {
		return nil, nil, errors.New("--vm-memory can't be used with --machine")
	}
	if flag.IsSpecified(ctx, "region") {
		return nil, nil, errors.New("--region can't be used with --machine")
	}

	flapsClient := flaps.FromContext(ctx)
	machineID := flag.GetString(ctx, "machine")
	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return nil, nil, err
	}
	if machine.State != api.MachineStateStarted {
		return nil, nil, fmt.Errorf("machine %s is not started", machineID)
	}
	if machine.IsFlyAppsReleaseCommand() {
		return nil, nil, fmt.Errorf("machine %s is a release command machine", machineID)
	}

	return machine, nil, nil
}

func makeEphemeralConsoleMachine(ctx context.Context, app *api.AppCompact, appConfig *appconfig.Config, guest *api.MachineGuest) (*api.Machine, func(), error) {
	apiClient := client.FromContext(ctx).API()
	currentRelease, err := apiClient.GetAppCurrentReleaseMachines(ctx, app.Name)
	if err != nil {
		return nil, nil, err
	}
	if currentRelease == nil {
		return nil, nil, errors.New("can't create an ephemeral console machine since the app has not yet been released")
	}

	machConfig, err := appConfig.ToConsoleMachineConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ephemeral console machine configuration: %w", err)
	}
	machConfig.Image = currentRelease.ImageRef
	machConfig.Guest = guest

	input := &machine.EphemeralInput{
		LaunchInput: api.LaunchMachineInput{
			Config:           machConfig,
			Region:           config.FromContext(ctx).Region,
			HostDedicationID: appConfig.HostDedicationID,
		},
		What: "to run the console",
	}
	return machine.LaunchEphemeral(ctx, input)
}

func determineEphemeralConsoleMachineGuest(ctx context.Context) (*api.MachineGuest, error) {
	desiredGuest := helpers.Clone(api.MachinePresets["shared-cpu-1x"])
	if flag.IsSpecified(ctx, "vm-size") {
		if err := desiredGuest.SetSize(flag.GetString(ctx, "vm-size")); err != nil {
			return nil, err
		}
	}

	if cpuKind := flag.GetString(ctx, "vm-cpukind"); cpuKind != "" {
		desiredGuest.CPUKind = cpuKind
	}

	if flag.IsSpecified(ctx, "vm-cpus") {
		cpus := flag.GetInt(ctx, "vm-cpus")
		var maxCPUs int
		switch desiredGuest.CPUKind {
		case "shared":
			maxCPUs = 8
		case "performance":
			maxCPUs = 16
		default:
			return nil, fmt.Errorf("invalid CPU kind '%s'", desiredGuest.CPUKind)
		}
		if cpus <= 0 || cpus > maxCPUs || (cpus != 1 && cpus%2 != 0) {
			return nil, errors.New("invalid number of CPUs")
		}
		desiredGuest.CPUs = cpus
	}
	cpuS := lo.Ternary(desiredGuest.CPUs == 1, "", "s")

	var minMemory, maxMemory, memoryIncrement int
	switch desiredGuest.CPUKind {
	case "shared":
		minMemory = desiredGuest.CPUs * api.MIN_MEMORY_MB_PER_SHARED_CPU
		maxMemory = desiredGuest.CPUs * api.MAX_MEMORY_MB_PER_SHARED_CPU
		memoryIncrement = 256
	case "performance":
		minMemory = desiredGuest.CPUs * api.MIN_MEMORY_MB_PER_CPU
		maxMemory = desiredGuest.CPUs * api.MAX_MEMORY_MB_PER_CPU
		memoryIncrement = 1024
	default:
		return nil, fmt.Errorf("invalid CPU kind '%s'", desiredGuest.CPUKind)
	}

	if flag.IsSpecified(ctx, "vm-memory") {
		memory := flag.GetInt(ctx, "vm-memory")
		switch {
		case memory < minMemory:
			return nil, fmt.Errorf("not enough memory (at least %d MB is required for %d %s CPU%s)", minMemory, desiredGuest.CPUs, desiredGuest.CPUKind, cpuS)
		case memory > maxMemory:
			return nil, fmt.Errorf("too much memory (at most %d MB is allowed for %d %s CPU%s)", maxMemory, desiredGuest.CPUs, desiredGuest.CPUKind, cpuS)
		case memory%memoryIncrement != 0:
			return nil, fmt.Errorf("memory must be in increments of %d MB", memoryIncrement)
		}
		desiredGuest.MemoryMB = memory
	} else {
		adjusted := lo.Clamp(desiredGuest.MemoryMB, minMemory, maxMemory)
		if adjusted != desiredGuest.MemoryMB && flag.IsSpecified(ctx, "vm-size") {
			action := lo.Ternary(adjusted < desiredGuest.MemoryMB, "lowered", "raised")
			terminal.Warnf("Ephemeral machine memory will be %s to %d MB to be compatible with %d %s CPU%s.\n", action, adjusted, desiredGuest.CPUs, desiredGuest.CPUKind, cpuS)
		}
		desiredGuest.MemoryMB = adjusted
	}

	return desiredGuest, nil
}
