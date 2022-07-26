package machine

import (
	"context"
	"fmt"

	"github.com/google/shlex"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newUpdate() *cobra.Command {
	const (
		short = "Update a machine"
		long  = short + "\n"

		usage = "update [machine_id]"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.Region(),
		flag.AppConfig(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Machine name, will be generated if missing",
		},
		flag.StringSlice{
			Name:        "port",
			Shorthand:   "p",
			Description: "Exposed port mappings (format: edgePort[:machinePort]/[protocol[:handler]])",
		},
		flag.Int{
			Name:        "cpus",
			Description: "Number of CPUs",
			Hidden:      true,
		},
		flag.Int{
			Name:        "memory",
			Description: "Memory (in megabytes) to attribute to the machine",
			Hidden:      true,
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
			Hidden:      true,
		},
		flag.String{
			Name:        "entrypoint",
			Description: "ENTRYPOINT replacement",
			Hidden:      true,
		},
		flag.String{
			Name:        "image",
			Shorthand:   "i",
			Description: "Docker Image to update the machine with",
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
			Hidden:      true,
		},
		flag.StringSlice{
			Name:        "metadata",
			Shorthand:   "m",
			Description: "Metadata in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		appName = app.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
	)

	machineID := flag.FirstArg(ctx)

	app, err := appFromMachineOrName(ctx, machineID, appName)

	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}

	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Machine %s was found and is currently in a %s state, attempting to update...\n", machineID, machine.State)

	input := api.LaunchMachineInput{
		ID:     machine.ID,
		AppID:  app.Name,
		Name:   machine.Name,
		Region: machine.Region,
	}

	machineConf := *machine.Config

	if cpus := flag.GetInt(ctx, "cpus"); cpus != 0 {
		machineConf.Guest.CPUs = cpus
	}

	if memory := flag.GetInt(ctx, "memory"); memory != 0 {
		machineConf.Guest.MemoryMB = memory
	}

	machineConf.Env, err = parseEnvVars(ctx)
	if err != nil {
		return err
	}

	services, err := determineServices(ctx)
	if err != nil {
		return err
	}
	if len(services) > 0 {
		machineConf.Services = services
	}

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

	machineConf.Mounts, err = determineMounts(ctx)
	if err != nil {
		return err
	}

	image := machine.FullImageRef()

	if flag.GetString(ctx, "image") != "" {
		image = flag.GetString(ctx, "image")
	}

	img, err := determineImage(ctx, app.Name, image)
	if err != nil {
		return err
	}
	machineConf.Image = img.Tag

	if flag.GetBool(ctx, "build-only") {
		return nil
	}

	input.Config = &machineConf

	machine, err = flapsClient.Update(ctx, input, "")

	if err != nil {
		return err
	}

	// wait for machine to be started
	if err := WaitForStart(ctx, flapsClient, machine); err != nil {
		return err
	}

	return nil
}
