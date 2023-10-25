package machine

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/google/shlex"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/watch"
)

var sharedFlags = flag.Set{
	flag.App(),
	flag.AppConfig(),
	flag.Detach(),
	flag.StringSlice{
		Name:      "port",
		Shorthand: "p",
		Description: `Publish ports, format: port[:machinePort][/protocol[:handler[:handler...]]])
	i.e.: --port 80/tcp --port 443:80/tcp:http:tls --port 5432/tcp:pg_tls
	To remove a port mapping use '-' as handler, i.e.: --port 80/tcp:-`,
	},
	flag.StringArray{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
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
		Name:        "build-nixpacks",
		Description: "Build your image with nixpacks",
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
	flag.StringArray{
		Name:        "kernel-arg",
		Description: "List of kernel arguments to be provided to the init. Can be specified multiple times.",
	},
	flag.StringArray{
		Name:        "metadata",
		Shorthand:   "m",
		Description: "Metadata in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
	flag.String{
		Name:        "schedule",
		Description: `Schedule a machine run at hourly, daily and monthly intervals`,
	},
	flag.Bool{
		Name:        "skip-dns-registration",
		Description: "Do not register the machine's 6PN IP with the internal DNS system",
	},
	flag.Bool{
		Name:        "autostart",
		Description: "Automatically start a stopped machine when a network request is received",
		Default:     true,
	},
	flag.Bool{
		Name:        "autostop",
		Description: "Automatically stop a machine when there aren't network requests for it",
		Default:     true,
	},
	flag.String{
		Name: "restart",
		Description: `Set the restart policy for a Machine. Options include 'no', 'always', and 'on-fail'.
	Default is 'on-fail' for Machines created by 'fly deploy' and Machines with a schedule. Default is 'always' for Machines created by 'fly m run'.`,
	},
	flag.StringSlice{
		Name:        "standby-for",
		Description: "Comma separated list of machine ids to watch for",
	},
	flag.StringArray{
		Name:        "file-local",
		Description: "Set of files in the form of /path/inside/machine=<local/path> pairs. Can be specified multiple times.",
	},
	flag.StringArray{
		Name:        "file-literal",
		Description: "Set of literals in the form of /path/inside/machine=VALUE pairs where VALUE is the content. Can be specified multiple times.",
	},
	flag.StringArray{
		Name:        "file-secret",
		Description: "Set of secrets in the form of /path/inside/machine=SECRET pairs where SECRET is the name of the secret. Can be specified multiple times.",
	},
	flag.VMSizeFlags,
}

func soManyErrors(args ...interface{}) error {
	sb := &strings.Builder{}
	errs := 0

	for i := range args {
		if i%2 == 0 {
			var err error

			kind := args[i].(string)
			erri := args[i+1]

			if erri != nil {
				err = erri.(error)
			}

			if err != nil {
				fmt.Fprintf(sb, "\t%s: %s\n", kind, err)
				errs += 1
			}
		}
	}

	if errs == 0 {
		return nil
	}

	if errs == 1 {
		return errors.New(strings.ReplaceAll(strings.ReplaceAll(sb.String(), "\t", ""), "\n", ""))
	}

	return fmt.Errorf("Multiple errors:\n%s", sb.String())
}

var s = spinner.New(spinner.CharSets[9], 100*time.Millisecond)

func newRun() *cobra.Command {
	const (
		short = "Run a machine"
		long  = short + "\n"

		usage = "run <image> [command]"
	)

	cmd := command.New(usage, short, long, runMachineRun,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.Region(),
		// deprecated in favor of `flyctl machine update`
		flag.String{
			Name:        "id",
			Description: "Machine ID, if previously known",
		},
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Machine name, will be generated if missing",
		},
		flag.String{
			Name:        "org",
			Description: `The organization that will own the app`,
		},
		flag.Bool{
			Name:        "rm",
			Description: "Automatically remove the machine when it exits",
		},
		flag.StringSlice{
			Name:        "volume",
			Shorthand:   "v",
			Description: "Volumes to mount in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
		},
		flag.Bool{
			Name:        "lsvd",
			Description: "Enable LSVD for this machine",
			Hidden:      true,
		},
		flag.String{
			Name:        "user",
			Description: "Username, if we're shelling into the machine now.",
			Default:     "root",
			Hidden:      false,
		},
		flag.String{
			Name:        "command",
			Description: "Command to run, if we're shelling into the machine now (in case you don't have bash).",
			Default:     "/bin/bash",
			Hidden:      false,
		},
		flag.Bool{
			Name:        "shell",
			Description: "Open a shell on the machine once created (implies --it --rm)",
			Hidden:      false,
		},

		sharedFlags,
	)

	cmd.Args = cobra.MinimumNArgs(0)

	return cmd
}

func runMachineRun(ctx context.Context) error {
	var (
		appName  = appconfig.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		err      error
		app      *api.AppCompact
		interact = false
		shell    = flag.GetBool(ctx, "shell")
		destroy  = flag.GetBool(ctx, "rm")
	)

	if shell {
		destroy = true
		interact = true
	}

	switch {
	case interact && appName != "":
		app, err = client.GetAppCompact(ctx, appName)
		if err != nil {
			return err
		}

	case interact && appName == "":
		app, err = getOrCreateEphemeralShellApp(ctx, client)
		if err != nil {
			return err
		}

	case appName == "":
		app, err = createApp(ctx, "Running a machine without specifying an app will create one for you, is this what you want?", "", client)
		if err != nil {
			return err
		}

		if app == nil {
			return nil
		}

	default:
		app, err = client.GetAppCompact(ctx, appName)
		if err != nil && strings.Contains(err.Error(), "Could not find App") {
			app, err = createApp(ctx, fmt.Sprintf("App '%s' does not exist, would you like to create it?", appName), appName, client)
			if err != nil {
				return err
			}

			if app == nil {
				return nil
			}
		}
		if err != nil {
			return err
		}
	}

	machineConf := &api.MachineConfig{
		AutoDestroy: destroy,
		DNS: &api.DNSConfig{
			SkipRegistration: flag.GetBool(ctx, "skip-dns-registration"),
		},
	}

	input := api.LaunchMachineInput{
		Name:   flag.GetString(ctx, "name"),
		Region: flag.GetString(ctx, "region"),
		LSVD:   flag.GetBool(ctx, "lsvd"),
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	if app.PlatformVersion == "nomad" {
		return fmt.Errorf("the app %s uses an earlier version of the platform that does not support machines", app.Name)
	}

	imageOrPath := flag.FirstArg(ctx)
	if imageOrPath == "" && shell {
		imageOrPath = "ubuntu"
	} else if imageOrPath == "" {
		return fmt.Errorf("image argument can't be an empty string")
	}

	machineID := flag.GetString(ctx, "id")
	if machineID != "" {
		return fmt.Errorf("to update an existing machine, use 'flyctl machine update'")
	}

	machineConf, err = determineMachineConfig(ctx, &determineMachineConfigInput{
		initialMachineConf: *machineConf,
		appName:            app.Name,
		imageOrPath:        imageOrPath,
		region:             input.Region,
		updating:           false,
		interact:           true,
	})
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "build-only") {
		return nil
	}

	input.SkipLaunch = len(machineConf.Standbys) > 0
	input.Config = machineConf

	machine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("could not launch machine: %w", err)
	}

	id, instanceID, state, privateIP := machine.ID, machine.InstanceID, machine.State, machine.PrivateIP

	fmt.Fprintf(io.Out, "Success! A machine has been successfully launched in app %s\n", app.Name)
	fmt.Fprintf(io.Out, " Machine ID: %s\n", id)

	if !interact {
		fmt.Fprintf(io.Out, " Instance ID: %s\n", instanceID)
		fmt.Fprintf(io.Out, " State: %s\n", state)
	}

	if input.SkipLaunch {
		return nil
	}

	if !interact {
		fmt.Fprintf(io.Out, "\n Attempting to start machine...\n\n")
	}

	s.Start()
	// wait for machine to be started
	err = mach.WaitForStartOrStop(ctx, machine, "start", time.Minute*5)
	s.Stop()
	if err != nil {
		return err
	}

	if interact {
		_, dialer, err := ssh.BringUpAgent(ctx, client, app, false)
		if err != nil {
			return err
		}

		// the app handle we have from creating a new app, presuming that's what
		// we did, doesn't have the ID set.
		app, err = client.GetAppCompact(ctx, app.Name)
		if err != nil {
			return fmt.Errorf("failed to load app info for %s: %w", app.Name, err)
		}

		sshClient, err := ssh.Connect(&ssh.ConnectParams{
			Ctx:            ctx,
			Org:            app.Organization,
			Dialer:         dialer,
			Username:       flag.GetString(ctx, "user"),
			DisableSpinner: false,
		}, machine.PrivateIP)
		if err != nil {
			return err
		}

		err = ssh.Console(ctx, sshClient, flag.GetString(ctx, "command"), true)
		if destroy {
			err = soManyErrors("console", err, "destroy machine", Destroy(ctx, app, machine, true))
		}

		if err != nil {
			return err
		}

		if destroy {
			return nil
		}
	}

	if !flag.GetDetach(ctx) {
		fmt.Fprintln(io.Out, colorize.Green("==> "+"Monitoring health checks"))

		if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
			return err
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintf(io.Out, "Machine started, you can connect via the following private ip\n")
	fmt.Fprintf(io.Out, "  %s\n", privateIP)

	return nil
}

func getOrCreateEphemeralShellApp(ctx context.Context, client *api.Client) (*api.AppCompact, error) {
	// no prompt if --org, buried in the context code
	org, err := prompt.Org(ctx)
	if err != nil {
		return nil, fmt.Errorf("create interactive shell app: %w", err)
	}

	apps, err := client.GetAppsForOrganization(ctx, org.ID)
	if err != nil {
		return nil, fmt.Errorf("create interactive shell app: %w", err)
	}

	var appc *api.App

	for appi, appt := range apps {
		if strings.HasPrefix(appt.Name, "flyctl-interactive-shells-") {
			appc = &apps[appi]
			break
		}
	}

	if appc == nil {
		appc, err = client.CreateApp(ctx, api.CreateAppInput{
			OrganizationID: org.ID,
			// i'll never find love again like the kind you give like the kind you send
			Name: fmt.Sprintf("flyctl-interactive-shells-%s-%d", org.ID, rand.Intn(1_000_000)),
		})

		if err != nil {
			return nil, fmt.Errorf("create interactive shell app: %w", err)
		}
	}

	// this app handle won't have all the metadata attached, so grab it
	app, err := client.GetAppCompact(ctx, appc.Name)
	if err != nil {
		return nil, err
	}

	return app, nil
}

func createApp(ctx context.Context, message, name string, client *api.Client) (*api.AppCompact, error) {
	confirm, err := prompt.Confirm(ctx, message)
	if err != nil {
		return nil, err
	}

	if !confirm {
		return nil, nil
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		return nil, err
	}

	if name == "" {
		name, err = selectAppName(ctx)
		if err != nil {
			return nil, err
		}
	}

	input := api.CreateAppInput{
		Name:           name,
		OrganizationID: org.ID,
	}

	app, err := client.CreateApp(ctx, input)
	if err != nil {
		return nil, err
	}

	return &api.AppCompact{
		ID:       app.ID,
		Name:     app.Name,
		Status:   app.Status,
		Deployed: app.Deployed,
		Hostname: app.Hostname,
		AppURL:   app.AppURL,
		Organization: &api.OrganizationBasic{
			ID:   app.Organization.ID,
			Slug: app.Organization.Slug,
		},
	}, nil
}

func parseKVFlag(ctx context.Context, flagName string, initialMap map[string]string) (parsed map[string]string, err error) {
	parsed = initialMap

	if value := flag.GetStringArray(ctx, flagName); len(value) > 0 {
		parsed, err = cmdutil.ParseKVStringsToMap(value)
		if err != nil {
			return nil, fmt.Errorf("invalid key/value pairs specified for flag %s", flagName)
		}
	}
	return parsed, nil
}

func selectAppName(ctx context.Context) (name string, err error) {
	const msg = "App Name:"

	if err = prompt.String(ctx, &name, msg, "", false); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("name argument or flag must be specified when not running interactively")
	}

	return
}

type determineMachineConfigInput struct {
	initialMachineConf api.MachineConfig
	appName            string
	imageOrPath        string
	region             string
	updating           bool
	interact           bool
}

func determineMachineConfig(
	ctx context.Context,
	input *determineMachineConfigInput) (*api.MachineConfig, error) {
	machineConf := mach.CloneConfig(&input.initialMachineConf)

	var err error
	machineConf.Guest, err = flag.GetMachineGuest(ctx, machineConf.Guest)
	if err != nil {
		return nil, err
	}

	if len(flag.GetStringArray(ctx, "kernel-arg")) != 0 {
		machineConf.Guest.KernelArgs = flag.GetStringArray(ctx, "kernel-arg")
	}

	parsedEnv, err := parseKVFlag(ctx, "env", machineConf.Env)
	if err != nil {
		return machineConf, err
	}

	if machineConf.Env == nil {
		machineConf.Env = make(map[string]string)
	}

	for k, v := range parsedEnv {
		machineConf.Env[k] = v
	}

	if flag.GetString(ctx, "schedule") != "" {
		machineConf.Schedule = flag.GetString(ctx, "schedule")
	}

	if input.updating {
		// Called from `update`. Command is specified by flag.
		if command := flag.GetString(ctx, "command"); command != "" {
			split, err := shlex.Split(command)
			if err != nil {
				return machineConf, errors.Wrap(err, "invalid command")
			}
			machineConf.Init.Cmd = split
		}
	} else {
		// Called from `run`. Command is specified by arguments.
		args := flag.Args(ctx)

		if len(args) != 0 {
			machineConf.Init.Cmd = args[1:]
		}
	}

	if input.interact {
		machineConf.Init.Exec = []string{"/bin/sleep", "inf"}
	}

	if flag.IsSpecified(ctx, "skip-dns-registration") {
		if machineConf.DNS == nil {
			machineConf.DNS = &api.DNSConfig{}
		}
		machineConf.DNS.SkipRegistration = flag.GetBool(ctx, "skip-dns-registration")
	}

	// Metadata
	parsedMetadata, err := parseKVFlag(ctx, "metadata", machineConf.Metadata)
	if err != nil {
		return machineConf, err
	}

	if machineConf.Metadata == nil {
		machineConf.Metadata = make(map[string]string)
	}

	for k, v := range parsedMetadata {
		machineConf.Metadata[k] = v
	}

	services, err := command.DetermineServices(ctx, machineConf.Services)
	if err != nil {
		return machineConf, err
	}
	machineConf.Services = services

	if entrypoint := flag.GetString(ctx, "entrypoint"); entrypoint != "" {
		splitted, err := shlex.Split(entrypoint)
		if err != nil {
			return machineConf, errors.Wrap(err, "invalid entrypoint")
		}
		machineConf.Init.Entrypoint = splitted
	}

	// default restart policy to always unless otherwise specified
	switch flag.GetString(ctx, "restart") {
	case "no":
		machineConf.Restart.Policy = api.MachineRestartPolicyNo
	case "on-fail":
		machineConf.Restart.Policy = api.MachineRestartPolicyOnFailure
	case "always":
		machineConf.Restart.Policy = api.MachineRestartPolicyAlways
	case "":
		if flag.IsSpecified(ctx, "restart") {
			// An empty policy was explicitly requested.
			machineConf.Restart.Policy = ""
		} else if !input.updating {
			// This is a new machine; apply the default.
			if machineConf.Schedule != "" {
				machineConf.Restart.Policy = api.MachineRestartPolicyOnFailure
			} else {
				machineConf.Restart.Policy = api.MachineRestartPolicyAlways
			}
		}
	default:
		return machineConf, errors.New("invalid restart provided")
	}

	machineConf.Mounts, err = command.DetermineMounts(ctx, machineConf.Mounts, input.region)
	if err != nil {
		return machineConf, err
	}

	if input.imageOrPath != "" {
		img, err := command.DetermineImage(ctx, input.appName, input.imageOrPath)
		if err != nil {
			return machineConf, err
		}
		machineConf.Image = img.Tag
	}

	// Service updates
	for idx := range machineConf.Services {
		s := &machineConf.Services[idx]
		// Use the chance to port the deprecated field
		if machineConf.DisableMachineAutostart != nil {
			s.Autostart = api.Pointer(!(*machineConf.DisableMachineAutostart))
			machineConf.DisableMachineAutostart = nil
		}

		if flag.IsSpecified(ctx, "autostop") {
			s.Autostop = api.Pointer(flag.GetBool(ctx, "autostop"))
		}

		if flag.IsSpecified(ctx, "autostart") {
			s.Autostart = api.Pointer(flag.GetBool(ctx, "autostart"))
		}
	}

	// Standby machine
	if flag.IsSpecified(ctx, "standby-for") {
		standbys := flag.GetStringSlice(ctx, "standby-for")
		machineConf.Standbys = lo.Ternary(len(standbys) > 0, standbys, nil)
	}

	machineFiles, err := command.FilesFromCommand(ctx)
	if err != nil {
		return machineConf, err
	}
	mach.MergeFiles(machineConf, machineFiles)

	return machineConf, nil
}
