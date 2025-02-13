package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/google/shlex"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

var sharedFlags = flag.Set{
	flag.App(),
	flag.AppConfig(),
	flag.Detach(),
	flag.StringSlice{
		Name:      "port",
		Shorthand: "p",
		Description: `The external ports and handlers for services, in the format: port[:machinePort][/protocol[:handler[:handler...]]])
	For example: --port 80/tcp --port 443:80/tcp:http:tls --port 5432/tcp:pg_tls
	To remove a port mapping use '-' as handler. For example: --port 80/tcp:-`,
	},
	flag.Env(),
	flag.String{
		Name:        "entrypoint",
		Description: "The command to override the Docker ENTRYPOINT.",
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
	},
	flag.Bool{
		Name:        "build-nixpacks",
		Description: "Build your image with nixpacks",
	},
	flag.String{
		Name:        "dockerfile",
		Description: "The path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
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
		Name:        "machine-config",
		Description: "Read machine config from json file or string",
	},
	flag.StringArray{
		Name:        "kernel-arg",
		Description: "A list of kernel arguments to provide to the init. Can be specified multiple times.",
	},
	flag.StringArray{
		Name:        "metadata",
		Shorthand:   "m",
		Description: "Metadata in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
	flag.String{
		Name:        "schedule",
		Description: `Schedule a Machine run at hourly, daily and monthly intervals`,
	},
	flag.Bool{
		Name:        "skip-dns-registration",
		Description: "Do not register the machine's 6PN IP with the internal DNS system",
	},
	flag.Bool{
		Name:        "autostart",
		Description: "Automatically start a stopped Machine when a network request is received",
		Default:     true,
	},
	flag.String{
		Name:        "autostop",
		Description: "Automatically stop a Machine when there are no network requests for it. Options include 'off', 'stop', and 'suspend'.",
		Default:     "off",
		NoOptDefVal: "stop",
	},
	flag.String{
		Name: "restart",
		Description: `Set the restart policy for a Machine. Options include 'no', 'always', and 'on-fail'.
	Default is 'on-fail' for Machines created by 'fly deploy' and Machines with a schedule. Default is 'always' for Machines created by 'fly m run'.`,
	},
	flag.StringSlice{
		Name:        "standby-for",
		Description: "For Machines without services, a comma separated list of Machine IDs to act as standby for.",
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
	flag.VMSizeFlags,
}

var runOrCreateFlags = flag.Set{
	flag.Region(),
	// deprecated in favor of `flyctl machine update`
	flag.String{
		Name:        "id",
		Description: "Machine ID, if previously known",
	},
	flag.String{
		Name:        "name",
		Shorthand:   "n",
		Description: "Machine name. Will be generated if omitted.",
	},
	flag.String{
		Name:        "org",
		Description: `The organization that will own the app`,
	},
	flag.Bool{
		Name:        "rm",
		Description: "Automatically remove the Machine when it exits. Sets the restart-policy to 'never' if not otherwise specified.",
	},
	flag.StringSlice{
		Name:        "volume",
		Shorthand:   "v",
		Description: "Volume to mount, in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
	},
	flag.Bool{
		Name:        "lsvd",
		Description: "Enable LSVD for this machine",
		Hidden:      true,
	},
	flag.Bool{
		Name:        "use-zstd",
		Description: "Enable zstd compression for the image",
	},
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
		runOrCreateFlags,
		sharedFlags,
		flag.Wireguard(),
		flag.String{
			Name:        "user",
			Description: "Used with --shell. The username, if we're shelling into the Machine now.",
			Default:     "root",
			Hidden:      false,
		},
		flag.String{
			Name:        "command",
			Description: "Used with --shell. The command to run, if we're shelling into the Machine now (in case you don't have bash).",
			Default:     "/bin/bash",
			Hidden:      false,
		},
		flag.Bool{
			Name:        "shell",
			Description: "Open a shell on the Machine once created (implies --it --rm). If no app is specified, a temporary app is created just for this Machine and destroyed when the Machine is destroyed. See also --command and --user.",
			Hidden:      false,
		},
	)

	cmd.Args = cobra.MinimumNArgs(0)

	return cmd
}

func newCreate() *cobra.Command {
	const (
		short = "Create, but don't start, a machine"
		long  = short + "\n"

		usage = "create <image> [command]"
	)

	cmd := command.New(usage, short, long, runMachineCreate,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		runOrCreateFlags,
		sharedFlags,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

type contextKey struct {
	name string
}

var createCommandCtxKey = &contextKey{"createCommand"}

func runMachineCreate(ctx context.Context) error {
	return runMachineRun(context.WithValue(ctx, createCommandCtxKey, true))
}

func runMachineRun(ctx context.Context) error {
	var (
		appName  = appconfig.NameFromContext(ctx)
		client   = flyutil.ClientFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		err      error
		app      *fly.AppCompact
		isCreate = false
		interact = false
		shell    = flag.GetBool(ctx, "shell")
		destroy  = flag.GetBool(ctx, "rm")
	)

	if shell {
		destroy = true
		interact = true
	}

	if ctx.Value(createCommandCtxKey) != nil {
		isCreate = true
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
		app, err = createApp(ctx, "Running a Machine without specifying an app will create one for you, is this what you want?", "", client)
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

	network, err := client.GetAppNetwork(ctx, app.Name)
	if err != nil {
		return err
	}

	machineConf := &fly.MachineConfig{
		AutoDestroy: destroy,
		DNS: &fly.DNSConfig{
			SkipRegistration: flag.GetBool(ctx, "skip-dns-registration"),
		},
	}

	input := fly.LaunchMachineInput{
		Name:   flag.GetString(ctx, "name"),
		Region: flag.GetString(ctx, "region"),
		LSVD:   flag.GetBool(ctx, "lsvd"),
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

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
		interact:           interact,
	})
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "build-only") {
		return nil
	}

	input.SkipLaunch = (len(machineConf.Standbys) > 0 || isCreate)
	input.Config = machineConf

	machine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("could not launch machine: %w", err)
	}

	id, instanceID, state, privateIP := machine.ID, machine.InstanceID, machine.State, machine.PrivateIP

	verb := "launched"
	if isCreate {
		verb = "created"
	}

	fmt.Fprintf(io.Out, "Success! A Machine has been successfully %s in app %s\n", verb, app.Name)
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
		_, dialer, err := ssh.BringUpAgent(ctx, client, app, *network, false)
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
			AppNames:       []string{app.Name},
		}, machine.PrivateIP)
		if err != nil {
			return err
		}

		err = ssh.Console(ctx, sshClient, flag.GetString(ctx, "command"), true, "")
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

		if err := watch.MachinesChecks(ctx, []*fly.Machine{machine}); err != nil {
			return err
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintf(io.Out, "Machine started, you can connect via the following private ip\n")
	fmt.Fprintf(io.Out, "  %s\n", privateIP)

	return nil
}

func getOrCreateEphemeralShellApp(ctx context.Context, client flyutil.Client) (*fly.AppCompact, error) {
	// no prompt if --org, buried in the context code
	org, err := prompt.Org(ctx)
	if err != nil {
		return nil, fmt.Errorf("create interactive shell app: %w", err)
	}

	apps, err := client.GetAppsForOrganization(ctx, org.ID)
	if err != nil {
		return nil, fmt.Errorf("create interactive shell app: %w", err)
	}

	var appc *fly.App

	for appi, appt := range apps {
		if strings.HasPrefix(appt.Name, "flyctl-interactive-shells-") {
			appc = &apps[appi]
			break
		}
	}

	if appc == nil {
		appc, err = client.CreateApp(ctx, fly.CreateAppInput{
			OrganizationID: org.ID,
			// i'll never find love again like the kind you give like the kind you send
			Name: fmt.Sprintf("flyctl-interactive-shells-%s-%d", strings.ToLower(org.ID), rand.Intn(1_000_000)),
		})

		if err != nil {
			return nil, fmt.Errorf("create interactive shell app: %w", err)
		}

		f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{AppName: appc.Name})
		if err != nil {
			return nil, err
		} else if err := f.WaitForApp(ctx, appc.Name); err != nil {
			return nil, err
		}
	}

	// this app handle won't have all the metadata attached, so grab it
	app, err := client.GetAppCompact(ctx, appc.Name)
	if err != nil {
		return nil, err
	}

	return app, nil
}

func createApp(ctx context.Context, message, name string, client flyutil.Client) (*fly.AppCompact, error) {
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

	input := fly.CreateAppInput{
		Name:           name,
		OrganizationID: org.ID,
	}

	app, err := client.CreateApp(ctx, input)
	if err != nil {
		return nil, err
	}

	f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{AppName: app.Name})
	if err != nil {
		return nil, err
	} else if err := f.WaitForApp(ctx, app.Name); err != nil {
		return nil, err
	}

	return &fly.AppCompact{
		ID:       app.ID,
		Name:     app.Name,
		Status:   app.Status,
		Deployed: app.Deployed,
		Hostname: app.Hostname,
		AppURL:   app.AppURL,
		Organization: &fly.OrganizationBasic{
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
	initialMachineConf fly.MachineConfig
	appName            string
	imageOrPath        string
	region             string
	updating           bool
	interact           bool
}

func determineMachineConfig(
	ctx context.Context,
	input *determineMachineConfigInput,
) (*fly.MachineConfig, error) {
	machineConf := mach.CloneConfig(&input.initialMachineConf)

	if emc := flag.GetString(ctx, "machine-config"); emc != "" {
		var buf []byte
		switch {
		case strings.HasPrefix(emc, "{"):
			buf = []byte(emc)
		case strings.HasSuffix(emc, ".json"):
			fo, err := os.Open(emc)
			if err != nil {
				return nil, err
			}
			buf, err = io.ReadAll(fo)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid machine config source: %q", emc)
		}

		if err := json.Unmarshal(buf, machineConf); err != nil {
			return nil, fmt.Errorf("invalid machine config %q: %w", emc, err)
		}
	}

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
		if flag.IsSpecified(ctx, "command") {
			command := strings.TrimSpace(flag.GetString(ctx, "command"))
			switch command {
			case "":
				machineConf.Init.Cmd = nil
			default:
				split, err := shlex.Split(command)
				if err != nil {
					return machineConf, errors.Wrap(err, "invalid command")
				}
				machineConf.Init.Cmd = split
			}
		}
	} else {
		// Called from `run`. Command is specified by arguments.
		args := flag.Args(ctx)

		if len(args) > 1 {
			machineConf.Init.Cmd = args[1:]
		} else if input.interact {
			machineConf.Init.Exec = []string{"/bin/sleep", "inf"}
		}
	}

	if flag.IsSpecified(ctx, "skip-dns-registration") {
		if machineConf.DNS == nil {
			machineConf.DNS = &fly.DNSConfig{}
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
		machineConf.Restart = &fly.MachineRestart{
			Policy: fly.MachineRestartPolicyNo,
		}
	case "on-fail":
		machineConf.Restart = &fly.MachineRestart{
			Policy: fly.MachineRestartPolicyOnFailure,
		}
	case "always":
		machineConf.Restart = &fly.MachineRestart{
			Policy: fly.MachineRestartPolicyAlways,
		}
	case "":
		if flag.IsSpecified(ctx, "restart") {
			// An empty policy was explicitly requested.
			machineConf.Restart = nil
		} else if machineConf.AutoDestroy {
			// Autodestroy only works when the restart policy is set to no, so unless otherwise specified, we set the restart policy to no.
			machineConf.Restart = &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo}
		} else if !input.updating {
			// This is a new machine; apply the default.
			if machineConf.Schedule != "" {
				machineConf.Restart = &fly.MachineRestart{
					Policy: fly.MachineRestartPolicyOnFailure,
				}
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
		machineConf.Image = img.String()
	}

	// Service updates
	for idx := range machineConf.Services {
		s := &machineConf.Services[idx]
		// Use the chance to port the deprecated field
		if machineConf.DisableMachineAutostart != nil {
			s.Autostart = fly.Pointer(!(*machineConf.DisableMachineAutostart))
			machineConf.DisableMachineAutostart = nil
		}

		if flag.IsSpecified(ctx, "autostop") {
			// We'll try to parse it as a boolean first for backward
			// compatibility. (strconv.ParseBool is what the pflag
			// library uses for booleans under the hood.)
			asString := flag.GetString(ctx, "autostop")
			if asBool, err := strconv.ParseBool(asString); err == nil {
				if asBool {
					s.Autostop = fly.Pointer(fly.MachineAutostopStop)
				} else {
					s.Autostop = fly.Pointer(fly.MachineAutostopOff)
				}
			} else {
				var value fly.MachineAutostop
				if err := value.UnmarshalText([]byte(asString)); err != nil {
					return nil, err
				}
				s.Autostop = fly.Pointer(value)
			}
		}

		if flag.IsSpecified(ctx, "autostart") {
			s.Autostart = fly.Pointer(flag.GetBool(ctx, "autostart"))
		}
	}

	// Standby machine
	if flag.IsSpecified(ctx, "standby-for") {
		standbys := flag.GetStringSlice(ctx, "standby-for")
		machineConf.Standbys = lo.Ternary(len(standbys) > 0, standbys, nil)
		machineConf.Env["FLY_STANDBY_FOR"] = strings.Join(standbys, ",")
	}

	machineFiles, err := command.FilesFromCommand(ctx)
	if err != nil {
		return machineConf, err
	}
	fly.MergeFiles(machineConf, machineFiles)

	return machineConf, nil
}
