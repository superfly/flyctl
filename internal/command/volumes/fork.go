package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newFork() *cobra.Command {
	const (
		long  = `Volume forking creates an independent copy of a storage volume for backup, testing, and experimentation without altering the original data.`
		short = "Forks the specified volume"
		usage = "fork <id>"
	)

	cmd := command.New(usage, short, long, runFork,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Name of the new volume",
		},
		flag.Bool{
			Name:        "machines-only",
			Description: "volume will be visible to machines platform only",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "require-unique-zone",
			Description: "Require volume to be placed in separate hardware zone from existing volumes",
		},
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runFork(ctx context.Context) error {
	var (
		cfg     = config.FromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
		volID   = flag.FirstArg(ctx)
	)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	vol, err := flapsClient.GetVolume(ctx, volID)
	if err != nil {
		return fmt.Errorf("failed to get volume: %w", err)
	}

	name := vol.Name
	if flag.IsSpecified(ctx, "name") {
		name = flag.GetString(ctx, "name")
	}

	var machinesOnly *bool
	if flag.IsSpecified(ctx, "machines-only") {
		machinesOnly = api.Pointer(flag.GetBool(ctx, "machines-only"))
	}

	var requireUniqueZone *bool
	if flag.IsSpecified(ctx, "require-unique-zone") {
		requireUniqueZone = api.Pointer(flag.GetBool(ctx, "require-unique-zone"))
	}

	input := api.CreateVolumeRequest{
		Name:              name,
		MachinesOnly:      machinesOnly,
		RequireUniqueZone: requireUniqueZone,
		SourceVolumeID:    &vol.ID,
	}

	volume, err := flapsClient.CreateVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to fork volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	if err := printVolume(out, volume, appName); err != nil {
		return err
	}

	return nil
}
