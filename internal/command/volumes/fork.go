package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newFork() *cobra.Command {
	const (
		long = `Volume forking is a feature that allows creating an independent copy of the specified storage volume for
		backup, testing, and experimentation purposes without altering the original data.`
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
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runFork(ctx context.Context) error {
	var (
		cfg     = config.FromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
		volID   = flag.FirstArg(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if app.IsPostgresApp() {
		return fmt.Errorf("This feature is not available for Postgres apps")
	}

	vol, err := client.GetVolume(ctx, volID)
	if err != nil {
		return fmt.Errorf("failed to get volume: %w", err)
	}

	input := api.ForkVolumeInput{
		AppID:          app.ID,
		SourceVolumeID: vol.ID,
		Name:           vol.Name,
		MachinesOnly:   app.PlatformVersion == "machines",
	}

	volume, err := client.ForkVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to fork volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	if err := printVolume(out, volume); err != nil {
		return err
	}

	return nil
}
