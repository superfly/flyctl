package volumes

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		short = "List the volumes associated with an app."

		long = short
	)

	cmd := command.New("list", short, long, runList,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	volumes, err := flapsClient.GetVolumes(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving volumes: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volumes)
	}

	return renderTable(ctx, volumes, app, out)
}
