package machine

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/shlex"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
)

var sharedFlags = flag.Set{
	flag.App(),
	flag.AppConfig(),
	flag.StringSlice{
		Name:        "port",
		Shorthand:   "p",
		Description: "Exposed port mappings (format: edgePort[:machinePort]/[protocol[:handler]])",
	},
	flag.String{
		Name:        "size",
		Shorthand:   "s",
		Description: "Preset guest cpu and memory for a machine, defaults to shared-cpu-1x",
	},
	flag.Int{
		Name:        "cpus",
		Description: "Number of CPUs",
	},
	flag.Int{
		Name:        "memory",
		Description: "Memory (in megabytes) to attribute to the machine",
	},
	flag.StringSlice{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
	flag.StringSlice{
		Name:        "volume",
		Shorthand:   "v",
		Description: "Volumes to mount in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
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
		Name:   "build-nixpacks",
		Hidden: true,
	},
	flag.String{
		Name:        "dockerfile",
		Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
	},
	flag.StringSlice{
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
	flag.StringSlice{
		Name:        "kernel-arg",
		Description: "List of kernel arguments to be provided to the init. Can be specified multiple times.",
	},
	flag.StringSlice{
		Name:        "metadata",
		Shorthand:   "m",
		Description: "Metadata in the form of NAME=VALUE pairs. Can be specified multiple times.",
	},
	flag.String{
		Name:        "schedule",
		Description: `Schedule a machine run at hourly, daily and monthly intervals`,
	},
}

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
		sharedFlags,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runMachineRun(ctx context.Context) error {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
		err     error
		app     *api.AppCompact
	)

	if appName == "" {
		app, err = createApp(ctx, "Running a machine without specifying an app will create one for you, is this what you want?", "", client)
		if err != nil {
			return err
		}
	} else {
		app, err = client.GetAppCompact(ctx, appName)
		if err != nil && strings.Contains(err.Error(), "Could not find App") {
			app, err = createApp(ctx, fmt.Sprintf("App '%s' does not exist, would you like to create it?", appName), appName, client)
			if app == nil {
				return nil
			}
		}
		if err != nil {
			return err
		}
	}

	machineConf := api.MachineConfig{
		Guest: &api.MachineGuest{
			CPUKind:    "shared",
			CPUs:       1,
			MemoryMB:   256,
			KernelArgs: flag.GetStringSlice(ctx, "kernel-arg"),
		},
	}

	input := api.LaunchMachineInput{
		AppID:  app.Name,
		Name:   flag.GetString(ctx, "name"),
		Region: flag.GetString(ctx, "region"),
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}

	machineID := flag.GetString(ctx, "id")
	if machineID != "" {
		return fmt.Errorf("to update an existing machine, use 'flyctl machine update'")
	}

	machineConf, err = determineMachineConfig(ctx, machineConf, app, flag.FirstArg(ctx))

	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "build-only") {
		return nil
	}

	input.Config = &machineConf

	machine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("could not launch machine: %w", err)
	}

	id, instanceID, state, privateIP := machine.ID, machine.InstanceID, machine.State, machine.PrivateIP

	fmt.Fprintf(io.Out, "Success! A machine has been successfully launched, waiting for it to be started\n")
	fmt.Fprintf(io.Out, " Machine ID: %s\n", id)
	fmt.Fprintf(io.Out, " Instance ID: %s\n", instanceID)
	fmt.Fprintf(io.Out, " State: %s\n", state)

	// wait for machine to be started
	if err := WaitForStartOrStop(ctx, flapsClient, machine, "start", time.Minute*5); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Machine started, you can connect via the following private ip\n")
	fmt.Fprintf(io.Out, "  %s\n", privateIP)

	return nil
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

func WaitForStartOrStop(ctx context.Context, flapsClient *flaps.Client, machine *api.Machine, action string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var waitOnAction string
	switch action {
	case "start":
		waitOnAction = "started"
	case "stop":
		waitOnAction = "stopped"
	default:
		return fmt.Errorf("action must be either start or stop")
	}

	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: false,
	}
	for {
		err := flapsClient.Wait(waitCtx, machine, waitOnAction)
		switch {
		case errors.Is(err, context.Canceled):
			return err
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("timeout reached waiting for machine to %s %w", waitOnAction, err)
		case err != nil:
			time.Sleep(b.Duration())
			continue
		}
		return nil
	}
}

func parseKVFlag(ctx context.Context, flagName string, initialMap map[string]string) (parsed map[string]string, err error) {
	parsed = initialMap

	if value := flag.GetStringSlice(ctx, flagName); len(value) > 0 {
		parsed, err = cmdutil.ParseKVStringsToMap(value)
		if err != nil {
			return nil, fmt.Errorf("invalid key/value pairs specified for flag %s", flagName)
		}
	}
	return parsed, nil
}

func determineImage(ctx context.Context, appName string, imageOrPath string) (img *imgsrc.DeploymentImage, err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	daemonType := imgsrc.NewDockerDaemonType(!flag.GetBool(ctx, "build-remote-only"), !flag.GetBool(ctx, "build-local-only"), env.IsCI(), flag.GetBool(ctx, "build-nixpacks"))
	resolver := imgsrc.NewResolver(daemonType, client, appName, io)

	// build if relative or absolute path
	if strings.HasPrefix(imageOrPath, ".") || strings.HasPrefix(imageOrPath, "/") {
		opts := imgsrc.ImageOptions{
			AppName:    appName,
			WorkingDir: path.Join(state.WorkingDirectory(ctx), imageOrPath),
			Publish:    !flag.GetBuildOnly(ctx),
			ImageLabel: flag.GetString(ctx, "image-label"),
			Target:     flag.GetString(ctx, "build-target"),
			NoCache:    flag.GetBool(ctx, "no-build-cache"),
		}
		if dockerfilePath := flag.GetString(ctx, "dockerfile"); dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return nil, err
			}
			opts.DockerfilePath = dockerfilePath
		}

		extraArgs, err := cmdutil.ParseKVStringsToMap(flag.GetStringSlice(ctx, "build-arg"))
		if err != nil {
			return nil, errors.Wrap(err, "invalid build-arg")
		}
		opts.BuildArgs = extraArgs

		img, err = resolver.BuildImage(ctx, io, opts)
		if err != nil {
			return nil, err
		}
		if img == nil {
			return nil, errors.New("could not find an image to deploy")
		}
	} else {
		opts := imgsrc.RefOptions{
			AppName:    appName,
			WorkingDir: state.WorkingDirectory(ctx),
			Publish:    !flag.GetBool(ctx, "build-only"),
			ImageRef:   imageOrPath,
			ImageLabel: flag.GetString(ctx, "image-label"),
		}

		img, err = resolver.ResolveReference(ctx, io, opts)
		if err != nil {
			return nil, err
		}
	}

	if img == nil {
		return nil, errors.New("could not find an image to deploy")
	}

	fmt.Fprintf(io.Out, "Image: %s\n", img.Tag)
	fmt.Fprintf(io.Out, "Image size: %s\n\n", humanize.Bytes(uint64(img.Size)))

	return img, nil
}

func determineMounts(ctx context.Context, mounts []api.MachineMount) ([]api.MachineMount, error) {
	for _, v := range flag.GetStringSlice(ctx, "volume") {
		splittedIDDestOpts := strings.Split(v, ":")

		mount := api.MachineMount{
			Volume: splittedIDDestOpts[0],
			Path:   splittedIDDestOpts[1],
		}

		if len(splittedIDDestOpts) > 2 {
			splittedOpts := strings.Split(splittedIDDestOpts[2], ",")
			for _, opt := range splittedOpts {
				splittedKeyValue := strings.Split(opt, "=")
				if splittedKeyValue[0] == "size" {
					i, err := strconv.Atoi(splittedKeyValue[1])
					if err != nil {
						return nil, errors.Wrapf(err, "could not parse volume '%s' size option value '%s', must be an integer", splittedIDDestOpts[0], splittedKeyValue[1])
					}
					mount.SizeGb = i
				} else if splittedKeyValue[0] == "encrypt" {
					mount.Encrypted = true
				}
			}
		}

		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func determineServices(ctx context.Context) ([]api.MachineService, error) {
	ports := flag.GetStringSlice(ctx, "port")

	if len(ports) <= 0 {
		return []api.MachineService{}, nil
	}

	machineServices := make([]api.MachineService, len(ports))

	for i, p := range flag.GetStringSlice(ctx, "port") {
		proto := "tcp"
		handlers := []string{}

		splittedPortsProto := strings.Split(p, "/")
		if len(splittedPortsProto) > 1 {
			splittedProtoHandlers := strings.Split(splittedPortsProto[1], ":")
			proto = splittedProtoHandlers[0]
			handlers = append(handlers, splittedProtoHandlers[1:]...)
		}

		splittedPorts := strings.Split(splittedPortsProto[0], ":")
		edgePort, err := strconv.Atoi(splittedPorts[0])
		if err != nil {
			return nil, errors.Wrap(err, "invalid edge port")
		}
		machinePort := edgePort
		if len(splittedPorts) > 1 {
			machinePort, err = strconv.Atoi(splittedPorts[1])
			if err != nil {
				return nil, errors.Wrap(err, "invalid machine (internal) port")
			}
		}

		machineServices[i] = api.MachineService{
			Protocol:     proto,
			InternalPort: machinePort,
			Ports: []api.MachinePort{
				{
					Port:     edgePort,
					Handlers: handlers,
				},
			},
		}
	}
	return machineServices, nil
}

func selectAppName(ctx context.Context) (name string, err error) {
	const msg = "App Name:"

	if err = prompt.String(ctx, &name, msg, "", false); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("name argument or flag must be specified when not running interactively")
	}

	return
}

func determineMachineConfig(ctx context.Context, initialMachineConf api.MachineConfig, app *api.AppCompact, image string) (machineConf api.MachineConfig, err error) {
	machineConf = initialMachineConf

	if guestSize := flag.GetString(ctx, "size"); guestSize != "" {
		guest, ok := api.MachinePresets[guestSize]
		if !ok {
			validSizes := []string{}
			for size := range api.MachinePresets {
				if strings.HasPrefix(size, "shared") {
					validSizes = append(validSizes, size)
				}
			}
			sort.Strings(validSizes)
			err = fmt.Errorf("invalid machine size requested, '%s', available:\n%s", guestSize, strings.Join(validSizes, "\n"))
			return
		}
		machineConf.Guest = guest
	} else {
		if cpus := flag.GetInt(ctx, "cpus"); cpus != 0 {
			machineConf.Guest.CPUs = cpus
		}

		if memory := flag.GetInt(ctx, "memory"); memory != 0 {
			machineConf.Guest.MemoryMB = memory
		}
	}

	machineConf.Env, err = parseKVFlag(ctx, "env", machineConf.Env)

	if err != nil {
		return
	}

	machineConf.Schedule = flag.GetString(ctx, "schedule")

	machineConf.Metadata, err = parseKVFlag(ctx, "metadata", machineConf.Metadata)

	if err != nil {
		return
	}

	services, err := determineServices(ctx)
	if err != nil {
		return
	}
	if len(services) > 0 {
		machineConf.Services = services
	}

	if entrypoint := flag.GetString(ctx, "entrypoint"); entrypoint != "" {
		splitted, err := shlex.Split(entrypoint)
		if err != nil {
			return machineConf, errors.Wrap(err, "invalid entrypoint")
		}
		machineConf.Init.Entrypoint = splitted
	}

	if cmd := flag.Args(ctx)[1:]; len(cmd) > 0 {
		machineConf.Init.Cmd = cmd
	}

	machineConf.Mounts, err = determineMounts(ctx, machineConf.Mounts)
	if err != nil {
		return
	}

	img, err := determineImage(ctx, app.Name, image)
	if err != nil {
		return
	}
	machineConf.Image = img.Tag

	return machineConf, nil
}
