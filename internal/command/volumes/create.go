package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newCreate() *cobra.Command {
	const (
		long = `Create new volume for app. --region flag must be included to specify
region the volume exists in. --size flag is optional, defaults to 10,
sets the size as the number of gigabytes the volume will consume.`

		short = "Create new volume for app"

		usage = "create <volumename>"
	)

	cmd := command.New(usage, short, long, runCreate,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Int{
			Name:        "size",
			Shorthand:   "s",
			Default:     3,
			Description: "Size of volume in gigabytes",
		},
		flag.Bool{
			Name:        "no-encryption",
			Description: "Do not encrypt the volume contents",
			Default:     false,
		},
		flag.Bool{
			Name:        "require-unique-zone",
			Description: "Require volume to be placed in separate hardware zone from existing volumes",
			Default:     true,
		},
	)

	return cmd
}

func runCreate(ctx context.Context) error {
	var (
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()

		volumeName = flag.FirstArg(ctx)
		appName    = app.NameFromContext(ctx)
	)

	appID, err := client.GetAppID(ctx, appName)
	if err != nil {
		return err
	}

	var region *api.Region

	if region, err = prompt.Region(ctx); err != nil {
		return err
	}

	input := api.CreateVolumeInput{
		AppID:             appID,
		Name:              volumeName,
		Region:            region.Code,
		SizeGb:            flag.GetInt(ctx, "size"),
		Encrypted:         !flag.GetBool(ctx, "no-encryption"),
		RequireUniqueZone: flag.GetBool(ctx, "require-unique-zone"),
	}

	volume, err := client.CreateVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed creating volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	return printVolume(out, volume)
}
