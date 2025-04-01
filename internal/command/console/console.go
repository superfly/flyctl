package console

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/docker/go-units"
	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
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
		flag.Wireguard(),
		flag.String{
			Name:        "machine",
			Description: "Run the console in the existing machine with the specified ID",
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Description: "Select the machine on which to execute the console from a list.",
			Default:     false,
		},
		flag.String{
			Name:        "container",
			Description: "Container to connect to",
		},
		flag.String{
			Name:        "user",
			Shorthand:   "u",
			Description: "Unix username to connect as",
			Default:     ssh.DefaultSshUsername,
		},
		flag.String{
			Name:        "image",
			Shorthand:   "i",
			Description: "image to use (default: current release)",
		},
		flag.Env(),
		flag.String{
			Name:        "entrypoint",
			Description: "ENTRYPOINT replacement",
		},
		flag.Bool{
			Name:        "build-only",
			Description: "Only build the image without running the machine",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "build-remote-only",
			Description: "Perform builds remotely without using the local docker daemon",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "build-local-only",
			Description: "Only perform builds locally using the local docker daemon",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "build-depot",
			Description: "Build your image with depot.dev",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "build-nixpacks",
			Description: "Build your image with nixpacks",
			Hidden:      true,
		},
		flag.String{
			Name:        "dockerfile",
			Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
		},
		flag.StringArray{
			Name:        "build-arg",
			Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
			Hidden:      true,
		},
		flag.String{
			Name:        "image-label",
			Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
			Hidden:      true,
		},
		flag.String{
			Name:        "build-target",
			Description: "Set the target build stage to build if the Dockerfile has more than one stage",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "no-build-cache",
			Description: "Do not use the cache when building the image",
			Hidden:      true,
		},
		flag.String{
			Name:        "command",
			Shorthand:   "C",
			Default:     "",
			Description: "command to run on SSH session",
		},
		flag.StringSlice{
			Name:      "port",
			Shorthand: "p",
			Description: `Publish ports, format: port[:machinePort][/protocol[:handler[:handler...]]])
		i.e.: --port 80/tcp --port 443:80/tcp:http:tls --port 5432/tcp:pg_tls
		To remove a port mapping use '-' as handler, i.e.: --port 80/tcp:-`,
		},
		flag.Bool{
			Name:        "skip-dns-registration",
			Description: "Do not register the machine's 6PN IP with the internal DNS system",
			Default:     true,
			Hidden:      true,
		},
		flag.StringArray{
			Name:        "file-local",
			Description: "Set of files to write to the Machine, in the form of /path/inside/machine=<local/path> pairs. Can be specified multiple times.",
		},
		flag.StringArray{
			Name:        "file-literal",
			Description: "Set of literals to write to the Machine, in the form of /path/inside/machine=VALUE pairs, where VALUE is the base64-encoded raw content. Can be specified multiple times.",
		},
		flag.StringArray{
			Name:        "file-secret",
			Description: "Set of secrets to write to the Machine, in the form of /path/inside/machine=SECRET pairs, where SECRET is the name of the secret. The content of the secret must be base64 encoded. Can be specified multiple times.",
		},
		flag.StringSlice{
			Name:        "volume",
			Description: "Volume to mount, in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
		},
		flag.VMSizeFlags,
	)

	return cmd
}

func runConsole(ctx context.Context) error {
	var (
		io        = iostreams.FromContext(ctx)
		appName   = appconfig.NameFromContext(ctx)
		apiClient = flyutil.ClientFromContext(ctx)
	)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app: %w", err)
	}

	network, err := apiClient.GetAppNetwork(ctx, app.Name)
	if err != nil {
		return fmt.Errorf("failed to get app network: %w", err)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to create flaps client: %w", err)
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig == nil {
		appConfig, err = appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to fetch app config from backend: %w", err)
		}
	}

	if err, extraInfo := appConfig.Validate(ctx); err != nil {
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

	_, dialer, err := ssh.BringUpAgent(ctx, apiClient, app, *network, false)
	if err != nil {
		return err
	}

	params := &ssh.ConnectParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		Username:       flag.GetString(ctx, "user"),
		DisableSpinner: false,
		Container:      flag.GetString(ctx, "container"),
		AppNames:       []string{app.Name},
	}
	sshClient, err := ssh.Connect(params, machine.PrivateIP)
	if err != nil {
		return err
	}

	consoleCommand := appConfig.ConsoleCommand

	if flag.IsSpecified(ctx, "command") {
		consoleCommand = flag.GetString(ctx, "command")
	}

	return ssh.Console(ctx, sshClient, consoleCommand, true, params.Container)
}

func selectMachine(ctx context.Context, app *fly.AppCompact, appConfig *appconfig.Config) (*fly.Machine, func(), error) {
	if flag.GetBool(ctx, "select") {
		return promptForMachine(ctx, app, appConfig)
	} else if flag.IsSpecified(ctx, "machine") {
		return getMachineByID(ctx)
	} else {
		guest, err := determineEphemeralConsoleMachineGuest(ctx, appConfig)
		if err != nil {
			return nil, nil, err
		}
		return makeEphemeralConsoleMachine(ctx, app, appConfig, guest)
	}
}

func promptForMachine(ctx context.Context, app *fly.AppCompact, appConfig *appconfig.Config) (*fly.Machine, func(), error) {
	if flag.IsSpecified(ctx, "machine") {
		return nil, nil, errors.New("--machine can't be used with -s/--select")
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return nil, nil, err
	}
	machines = lo.Filter(machines, func(machine *fly.Machine, _ int) bool {
		return machine.State == fly.MachineStateStarted
	})

	ephemeralGuest, err := determineEphemeralConsoleMachineGuest(ctx, appConfig)
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

func getMachineByID(ctx context.Context) (*fly.Machine, func(), error) {
	if flag.IsSpecified(ctx, "vm-cpus") {
		return nil, nil, errors.New("--vm-cpus can't be used with --machine")
	}
	if flag.IsSpecified(ctx, "vm-memory") {
		return nil, nil, errors.New("--vm-memory can't be used with --machine")
	}
	if flag.IsSpecified(ctx, "region") {
		return nil, nil, errors.New("--region can't be used with --machine")
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	machineID := flag.GetString(ctx, "machine")
	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return nil, nil, err
	}
	if machine.State != fly.MachineStateStarted {
		return nil, nil, fmt.Errorf("machine %s is not started", machineID)
	}
	if machine.IsFlyAppsReleaseCommand() {
		return nil, nil, fmt.Errorf("machine %s is a release command machine", machineID)
	}

	return machine, nil, nil
}

func makeEphemeralConsoleMachine(ctx context.Context, app *fly.AppCompact, appConfig *appconfig.Config, guest *fly.MachineGuest) (*fly.Machine, func(), error) {
	apiClient := flyutil.ClientFromContext(ctx)
	currentRelease, err := apiClient.GetAppCurrentReleaseMachines(ctx, app.Name)
	if err != nil {
		return nil, nil, err
	}

	if !flag.IsSpecified(ctx, "image") && flag.IsSpecified(ctx, "dockerfile") {
		flag.SetString(ctx, "image", ".")
	}

	if currentRelease == nil && !flag.IsSpecified(ctx, "image") {
		return nil, nil, errors.New("can't create an ephemeral console machine since the app has not yet been released")
	}

	machConfig, err := appConfig.ToConsoleMachineConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ephemeral console machine configuration: %w", err)
	}

	machConfig.Mounts, err = command.DetermineMounts(ctx, machConfig.Mounts, config.FromContext(ctx).Region)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to process mounts: %w", err)
	}

	if flag.IsSpecified(ctx, "image") {
		img, err := command.DetermineImage(ctx, app.Name, flag.GetString(ctx, "image"))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get image: %w", err)
		}
		machConfig.Image = img.Tag
	} else {
		machConfig.Image = currentRelease.ImageRef
	}

	if env := flag.GetStringArray(ctx, "env"); len(env) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(env)
		if err != nil {
			return nil, nil, fmt.Errorf("failed parsing environment: %w", err)
		}
		maps.Copy(machConfig.Env, parsedEnv)
	}

	if machConfig.DNS == nil {
		machConfig.DNS = &fly.DNSConfig{}
	}
	machConfig.DNS.SkipRegistration = flag.GetBool(ctx, "skip-dns-registration")

	machineFiles, err := command.FilesFromCommand(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed parsing files: %w", err)
	}
	fly.MergeFiles(machConfig, machineFiles)

	machConfig.Guest = guest

	services, err := command.DetermineServices(ctx, machConfig.Services)
	if err != nil {
		return nil, nil, fmt.Errorf("failed parsing port: %w", err)
	}
	machConfig.Services = services

	if entrypoint := flag.GetString(ctx, "entrypoint"); entrypoint != "" {
		splitted, err := shlex.Split(entrypoint)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid entrypoint: %w", err)
		}
		machConfig.Init.Entrypoint = splitted
	}

	if hdid := appConfig.HostDedicationID; hdid != "" {
		machConfig.Guest.HostDedicationID = hdid
	}

	input := &machine.EphemeralInput{
		LaunchInput: fly.LaunchMachineInput{
			Config: machConfig,
			Region: config.FromContext(ctx).Region,
		},
		What: "to run the console",
	}
	return machine.LaunchEphemeral(ctx, input)
}

func determineEphemeralConsoleMachineGuest(ctx context.Context, appConfig *appconfig.Config) (*fly.MachineGuest, error) {
	var guest *fly.MachineGuest

	if compute := appConfig.ComputeForGroup("console"); compute != nil {
		guest = compute.MachineGuest

		if compute.Memory != "" {
			mb, err := helpers.ParseSize(compute.Memory, units.RAMInBytes, units.MiB)
			if err != nil {
				return nil, fmt.Errorf("invalid memory size: %w", err)
			}
			if guest == nil {
				guest = &fly.MachineGuest{}
			}
			guest.MemoryMB = mb
		}
	}

	guest, err := flag.GetMachineGuest(ctx, guest)
	if err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var minMemory, maxMemory int
	switch guest.CPUKind {
	case "shared":
		minMemory = guest.CPUs * fly.MIN_MEMORY_MB_PER_SHARED_CPU
		maxMemory = guest.CPUs * fly.MAX_MEMORY_MB_PER_SHARED_CPU
	case "performance":
		minMemory = guest.CPUs * fly.MIN_MEMORY_MB_PER_CPU
		maxMemory = guest.CPUs * fly.MAX_MEMORY_MB_PER_CPU
	default:
		return nil, fmt.Errorf("invalid CPU kind '%s'; valid values are 'shared' and 'performance'", guest.CPUKind)
	}

	adjusted := lo.Clamp(guest.MemoryMB, minMemory, maxMemory)
	if adjusted != guest.MemoryMB {
		action := lo.Ternary(adjusted < guest.MemoryMB, "lowered", "raised")
		cpuS := lo.Ternary(guest.CPUs == 1, "", "s")
		terminal.Warnf(
			"Ephemeral machine memory will be %s from %d MB to %d MB to be compatible with %d %s CPU%s.",
			action,
			guest.MemoryMB,
			adjusted,
			guest.CPUs,
			guest.CPUKind,
			cpuS,
		)
	}
	guest.MemoryMB = adjusted

	return guest, nil
}
