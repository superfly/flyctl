package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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
		flag.Bool{
			Name:        "all",
			Description: "Show all volumes including those in destroyed states",
		},
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppBasic(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	var volumes []fly.Volume
	if flag.GetBool(ctx, "all") {
		volumes, err = flapsClient.GetAllVolumes(ctx)
	} else {
		volumes, err = flapsClient.GetVolumes(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed retrieving volumes: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volumes)
	}

	return renderTable(ctx, volumes, app, out, true)
}
