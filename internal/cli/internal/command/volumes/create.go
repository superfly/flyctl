package volumes

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/region"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newCreate() *cobra.Command {
	const (
		long = `Create new volume for app. --region flag must be included to specify
region the volume exists in. --size flag is optional, defaults to 10,
sets the size as the number of gigabytes the volume will consume.`

		short = "Create new volume for app"
	)

	cmd := command.New("create", short, long, runCreate,
		command.RequireSession,
		command.RequireAppName,
		command.RequireRegion,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Int{
			Name:        "size",
			Shorthand:   "s",
			Default:     10,
			Description: "Size of volume in gigabytes",
		},
		flag.Bool{
			Name:        "encrypted",
			Description: "Encrypt volume",
			Default:     true,
		},
		flag.Bool{
			Name:        "require-unique-zone",
			Description: "Require volume to be placed in separate hardware zone from existing volumes",
			Default:     true,
		},
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()

		volumeName = flag.FirstArg(ctx)
		appName    = app.NameFromContext(ctx)
	)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return
	}

	input := api.CreateVolumeInput{
		AppID:             app.ID,
		Name:              volumeName,
		Region:            region.FromContext(ctx),
		SizeGb:            flag.GetInt(ctx, "size"),
		Encrypted:         flag.GetBool(ctx, "encrypted"),
		RequireUniqueZone: flag.GetBool(ctx, "require-unique-zone"),
	}

	volume, err := client.CreateVolume(ctx, input)

	if err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	return printVolume(out, volume)
}
