package machine

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/google/shlex"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/pkg/flaps"
)

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
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.String{
			Name:        "id",
			Description: "Machine ID, is previously known",
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
		flag.StringSlice{
			Name:        "port",
			Shorthand:   "p",
			Description: "Exposed port mappings (format: edgePort[:machinePort]/[protocol[:handler]])",
		},
		flag.String{
			Name:        "size",
			Shorthand:   "s",
			Description: "Preset guest cpu and memory for a machine",
		},
		flag.String{
			Name:        "cpu-kind",
			Description: "Kind of CPU to use (shared, dedicated)",
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
			Name:        "detach",
			Shorthand:   "d",
			Description: "Detach from the machine's logs",
		},
		flag.Bool{
			Name: "build-only",
		},
		flag.Bool{
			Name:        "build-remote-only",
			Description: "Perform builds remotely without using the local docker daemon",
		},
		flag.Bool{
			Name:        "build-local-only",
			Description: "Only perform builds locally using the local docker daemon",
		},
		flag.String{
			Name:        "dockerfile",
			Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
		},
		flag.StringSlice{
			Name:        "build-arg",
			Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
		flag.String{
			Name:        "image-label",
			Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
		},
		flag.String{
			Name:        "build-target",
			Description: "Set the target build stage to build if the Dockerfile has more than one stage",
		},
		flag.Bool{
			Name:        "no-build-cache",
			Description: "Do not use the cache when building the image",
		},
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runMachineRun(ctx context.Context) error {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	var org *api.Organization

	var err error

	var app *api.App

	if appName == "" {
		var message = "Running a machine without specifying an app will create one for you, is this what you want?"

		confirm, err := prompt.Confirm(ctx, message)
		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}

		org, err = prompt.Org(ctx, nil)
		if err != nil {
			return err
		}

		name, err := selectAppName(ctx)
		if err != nil {
			return err
		}

		input := api.CreateAppInput{
			Name:           name,
			Runtime:        "FIRECRACKER",
			OrganizationID: org.ID,
		}

		app, err = client.CreateApp(ctx, input)
		if err != nil {
			return err
		}

	} else {
		app, err = client.GetApp(ctx, appName)
		if err != nil {
			return err
		}
	}

	var machineConf = new(api.MachineConfig)

	machineConf.Env, err = parseEnvVars(ctx)
	if err != nil {
		return err
	}

	img, err := determineImage(ctx, app.Name)
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "build-only") {
		return nil
	}

	machineConf.Image = img.Tag

	guest := api.MachinePresets[flag.GetString(ctx, "size")]

	if guest == nil {
		cpuKind := flag.GetString(ctx, "cpu-kind")
		if cpuKind == "" {
			cpuKind = "shared"
		}

		cpus := flag.GetInt(ctx, "cpus")
		if cpus == 0 {
			cpus = 1
		}

		memory := flag.GetInt(ctx, "memory")
		if memory == 0 {
			memory = 256
		}
		guest = &api.MachineGuest{
			CPUKind:  cpuKind,
			CPUs:     cpus,
			MemoryMB: memory,
		}
	} else {
		if cpuKind := flag.GetString(ctx, "cpu-kind"); cpuKind != "" {
			guest.CPUKind = cpuKind
		}
		if cpus := flag.GetInt(ctx, "cpus"); cpus != 0 {
			guest.CPUs = cpus
		}
		if memory := flag.GetInt(ctx, "memory"); memory != 0 {
			guest.MemoryMB = memory
		}
	}

	machineConf.Guest = guest

	if entrypoint := flag.GetString(ctx, "entrypoint"); entrypoint != "" {
		splitted, err := shlex.Split(entrypoint)
		if err != nil {
			return errors.Wrap(err, "invalid entrypoint")
		}
		machineConf.Init.Entrypoint = splitted
	}

	if cmd := flag.Args(ctx)[1:]; len(cmd) > 0 {
		machineConf.Init.Cmd = cmd
	}

	services, err := determineServices(ctx)
	if err != nil {
		return err
	}
	if len(services) > 0 {
		machineConf.Services = services
	}

	machineConf.Mounts, err = determineMounts(ctx)
	if err != nil {
		return err
	}

	input := api.LaunchMachineInput{
		AppID:  app.Name,
		ID:     flag.GetString(ctx, "id"),
		Name:   flag.GetString(ctx, "name"),
		Region: flag.GetString(ctx, "region"),
		Config: machineConf,
	}

	if org != nil {
		input.OrgSlug = org.ID
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	mach, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("could not launch machine: %w", err)
	}

	fmt.Println(mach)
	return nil

	// if flag.GetBool(ctx, "detach") {
	// 	fmt.Fprintf(io.Out, "Machine: %s\n", machine.ID)
	// 	return nil
	// }

	// // apiClient := cmdCtx.Client.API()

	// opts := &logs.LogOptions{
	// 	AppName: app.Name,
	// 	VMID:    mach.ID,
	// }

	// stream, err := logs.NewNatsStream(ctx, client, opts)

	// if err != nil {
	// 	terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
	// 	terminal.Debug("Falling back to log polling...")

	// 	stream, err = logs.NewPollingStream(ctx, client, opts)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// presenter := presenters.LogPresenter{}

	// entries := stream.Stream(ctx, opts)

	// for {
	// 	select {
	// 	case <-ctx.Done():
	// 		return stream.Err()
	// 	case entry := <-entries:
	// 		presenter.FPrint(io.Out, config.FromContext(ctx).JSONOutput, entry)
	// 	}
	// }
}

func parseEnvVars(ctx context.Context) (map[string]string, error) {
	var env = make(map[string]string)

	if extraEnv := flag.GetStringSlice(ctx, "env"); len(extraEnv) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(flag.GetStringSlice(ctx, "env"))
		if err != nil {
			return nil, errors.Wrap(err, "invalid env")
		}
		env = parsedEnv
	}
	return env, nil
}

func determineImage(ctx context.Context, appName string) (img *imgsrc.DeploymentImage, err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	daemonType := imgsrc.NewDockerDaemonType(!flag.GetBool(ctx, "build-remote-only"), !flag.GetBool(ctx, "build-local-only"))
	resolver := imgsrc.NewResolver(daemonType, client, appName, io)

	imageOrPath := flag.FirstArg(ctx)
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
		opts.ExtraBuildArgs = extraArgs

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
	fmt.Fprintf(io.Out, "Image size: %s\n", humanize.Bytes(uint64(img.Size)))

	return img, nil
}

func determineMounts(ctx context.Context) ([]api.MachineMount, error) {
	var mounts []api.MachineMount

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

func determineServices(ctx context.Context) ([]interface{}, error) {
	ports := flag.GetStringSlice(ctx, "port")

	if len(ports) <= 0 {
		return []interface{}{}, nil
	}

	svcs := make([]interface{}, len(ports))

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

		svcs[i] = map[string]interface{}{
			"protocol":      proto,
			"internal_port": machinePort,
			"ports": []map[string]interface{}{
				{
					"port":     edgePort,
					"handlers": handlers,
				},
			},
		}
	}
	return svcs, nil
}

func selectAppName(ctx context.Context) (name string, err error) {
	const msg = "App Name:"

	if err = prompt.String(ctx, &name, msg, "", false); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("name argument or flag must be specified when not running interactively")
	}

	return
}
