package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/shlex"
	"github.com/pkg/errors"
	"github.com/samber/lo"
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
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/watch"
)

var sharedFlags = flag.Set{
	flag.App(),
	flag.AppConfig(),
	flag.Detach(),
	flag.StringSlice{
		Name:        "port",
		Shorthand:   "p",
		Description: "Exposed port mappings (format: (edgePort|startPort-endPort)[:machinePort]/[protocol[:handler]])",
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
		Name:        "build-nixpacks",
		Description: "Build your image with nixpacks",
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
	flag.String{
		Name:        "machine-config",
		Description: `Path to a machine configuration json file.`,
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

	// If we pass a JSON machine configuration, we don't build a local image.
	// That introduces this ugliness, which changes the expected arg count depending
	// on flags passed.
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if mc := cmd.Flag("machine-config"); mc != nil && mc.Changed {
			if len(args) != 0 {
				return fmt.Errorf("expected no arguments when a machine config is present, received %d", len(args))
			}
			return nil
		}
		if len(args) == 0 {
			return fmt.Errorf("requires at least 1 arg, only received %d", len(args))
		}
		return nil
	}

	return cmd
}

func runMachineRun(ctx context.Context) (err error) {
	var (
		appName  = app.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		app      *api.AppCompact
	)

	var input api.LaunchMachineInput
	usingJsonInput := false

	// if we're passing a JSON configuration, use that directly instead of deriving a configuration
	if configJsonPath := flag.GetString(ctx, "machine-config"); configJsonPath != "" {

		usingJsonInput = true

		if !filepath.IsAbs(configJsonPath) {
			absConfigJsonPath, err := filepath.Abs(filepath.Join(state.WorkingDirectory(ctx), configJsonPath))
			if err != nil {
				return err
			}
			configJsonPath = absConfigJsonPath
		}

		{
			file, err := os.Open(configJsonPath)
			if err != nil {
				return fmt.Errorf("failed to open config file: %w", err)
			}

			defer func() {
				if e := file.Close(); err == nil {
					err = e
				}
			}()

			if err = json.NewDecoder(file).Decode(&input); err != nil {

				return fmt.Errorf("failed parsing machine configuration: %w", err)
			}
		}

		if appName == "" {
			appName = input.AppID
		}
	}

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
	if app == nil {
		return nil
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	if app.PlatformVersion == "nomad" {
		return fmt.Errorf("the app %s uses an earlier version of the platform that does not support machines", app.Name)
	}

	input.AppID = app.Name
	if name := flag.GetString(ctx, "name"); name != "" {
		input.Name = name
	}
	if region := flag.GetString(ctx, "region"); region != "" {
		input.Region = region
	}

	// if we aren't passing a JSON configuration (which we usually aren't), derive a machine configuration
	if !usingJsonInput {

		machineConf := &api.MachineConfig{
			Guest: &api.MachineGuest{
				CPUKind:    "shared",
				CPUs:       1,
				MemoryMB:   256,
				KernelArgs: flag.GetStringSlice(ctx, "kernel-arg"),
			},
		}

		machineID := flag.GetString(ctx, "id")
		if machineID != "" {
			return fmt.Errorf("to update an existing machine, use 'flyctl machine update'")
		}

		machineConf, err = determineMachineConfig(ctx, *machineConf, app, flag.FirstArg(ctx), input.Region)
		if err != nil {
			return err
		}

		if flag.GetBool(ctx, "build-only") {
			return nil
		}

		input.Config = machineConf
	}

	// now actually launch the machine
	machine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("could not launch machine: %w", err)
	}

	id, instanceID, state, privateIP := machine.ID, machine.InstanceID, machine.State, machine.PrivateIP

	fmt.Fprintf(io.Out, "Success! A machine has been successfully launched in app %s, waiting for it to be started\n", app.Name)
	fmt.Fprintf(io.Out, " Machine ID: %s\n", id)
	fmt.Fprintf(io.Out, " Instance ID: %s\n", instanceID)
	fmt.Fprintf(io.Out, " State: %s\n", state)

	// wait for machine to be started
	if err := mach.WaitForStartOrStop(ctx, machine, "start", time.Minute*5); err != nil {
		return err
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
			WorkingDir: path.Join(state.WorkingDirectory(ctx)),
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

func determineMounts(ctx context.Context, mounts []api.MachineMount, region string) ([]api.MachineMount, error) {
	unattachedVolumes := make(map[string][]api.Volume)

	pathIndex := make(map[string]int)
	for idx, m := range mounts {
		pathIndex[m.Path] = idx
	}

	for _, v := range flag.GetStringSlice(ctx, "volume") {
		splittedIDDestOpts := strings.Split(v, ":")
		if len(splittedIDDestOpts) < 2 {
			return nil, fmt.Errorf("Can't infer volume and mount path from '%s'", v)
		}
		volID := splittedIDDestOpts[0]
		mountPath := splittedIDDestOpts[1]

		if !strings.HasPrefix(volID, "vol_") {
			volName := volID

			// Load app volumes the first time
			if len(unattachedVolumes) == 0 {
				var err error
				unattachedVolumes, err = getUnattachedVolumes(ctx, region)
				if err != nil {
					return nil, err
				}
			}

			if len(unattachedVolumes[volName]) == 0 {
				return nil, fmt.Errorf("not enough unattached volumes for '%s'", volName)
			}
			volID = unattachedVolumes[volName][0].ID
			unattachedVolumes[volName] = unattachedVolumes[volName][1:]
		}

		if idx, found := pathIndex[mountPath]; found {
			mounts[idx].Volume = volID
		} else {
			mounts = append(mounts, api.MachineMount{
				Volume: volID,
				Path:   mountPath,
			})
		}
	}
	return mounts, nil
}

func getUnattachedVolumes(ctx context.Context, regionCode string) (map[string][]api.Volume, error) {
	appName := app.NameFromContext(ctx)
	apiclient := client.FromContext(ctx).API()

	if regionCode == "" {
		region, err := apiclient.GetNearestRegion(ctx)
		if err != nil {
			return nil, err
		}
		regionCode = region.Code
	}

	volumes, err := apiclient.GetVolumes(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("Error fetching application volumes: %w", err)
	}

	unattached := lo.Filter(volumes, func(v api.Volume, _ int) bool {
		return !v.IsAttached() && (regionCode == v.Region)
	})
	if len(unattached) == 0 {
		return nil, fmt.Errorf("No unattached volumes in region '%s'", regionCode)
	}

	unattachedMap := lo.GroupBy(unattached, func(v api.Volume) string { return v.Name })
	return unattachedMap, nil
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
		if len(splittedPortsProto) == 2 {
			splittedProtoHandlers := strings.Split(splittedPortsProto[1], ":")
			proto = splittedProtoHandlers[0]
			handlers = append(handlers, splittedProtoHandlers[1:]...)
		} else if len(splittedPortsProto) > 2 {
			return nil, errors.New("port must be at most two elements (ports/protocol:handler)")
		}

		edgePort, edgeStartPort, edgeEndPort, internalPort, err := parsePorts(splittedPortsProto[0])
		if err != nil {
			return nil, err
		}

		machineServices[i] = api.MachineService{
			Protocol:     proto,
			InternalPort: internalPort,
			Ports: []api.MachinePort{
				{
					Port:      edgePort,
					StartPort: edgeStartPort,
					EndPort:   edgeEndPort,
					Handlers:  handlers,
				},
			},
		}
	}
	return machineServices, nil
}

func parsePorts(input string) (port, start_port, end_port *int32, internal_port int, err error) {
	split := strings.Split(input, ":")
	if len(split) == 1 {
		var external_port int
		external_port, err = strconv.Atoi(split[0])
		if err != nil {
			err = errors.Wrap(err, "invalid port")
			return
		}

		p := int32(external_port)
		port = &p
	} else if len(split) == 2 {
		internal_port, err = strconv.Atoi(split[1])
		if err != nil {
			err = errors.Wrap(err, "invalid machine (internal) port")
			return
		}

		external_split := strings.Split(split[0], "-")
		if len(external_split) == 1 {
			var external_port int
			external_port, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid external port")
				return
			}

			p := int32(external_port)
			port = &p
		} else if len(external_split) == 2 {
			var start int
			start, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid start port for port range")
				return
			}

			s := int32(start)
			start_port = &s

			var end int
			end, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid end port for port range")
				return
			}

			e := int32(end)
			end_port = &e
		} else {
			err = errors.New("external port must be at most 2 elements (port, or range start-end)")
		}
	} else {
		err = errors.New("port definition must be at most 2 elements (external:internal)")
	}

	return
}

func selectAppName(ctx context.Context) (name string, err error) {
	const msg = "App Name:"

	if err = prompt.String(ctx, &name, msg, "", false); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("name argument or flag must be specified when not running interactively")
	}

	return
}

func determineMachineConfig(ctx context.Context, initialMachineConf api.MachineConfig, app *api.AppCompact, imageOrPath string, region string) (*api.MachineConfig, error) {
	machineConf, err := mach.CloneConfig(initialMachineConf)
	if err != nil {
		return nil, err
	}

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
			return machineConf, fmt.Errorf("invalid machine size requested, '%s', available:\n%s", guestSize, strings.Join(validSizes, "\n"))
		}
		guest.KernelArgs = machineConf.Guest.KernelArgs
		machineConf.Guest = guest
	}

	// Potential overrides for Guest
	if cpus := flag.GetInt(ctx, "cpus"); cpus != 0 {
		machineConf.Guest.CPUs = cpus
	}

	if memory := flag.GetInt(ctx, "memory"); memory != 0 {
		machineConf.Guest.MemoryMB = memory
	}

	if len(flag.GetStringSlice(ctx, "kernel-arg")) != 0 {
		machineConf.Guest.KernelArgs = flag.GetStringSlice(ctx, "kernel-arg")
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

	services, err := determineServices(ctx)
	if err != nil {
		return machineConf, err
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

	machineConf.Mounts, err = determineMounts(ctx, machineConf.Mounts, region)
	if err != nil {
		return machineConf, err
	}

	img, err := determineImage(ctx, app.Name, imageOrPath)
	if err != nil {
		return machineConf, err
	}
	machineConf.Image = img.Tag

	return machineConf, nil
}
