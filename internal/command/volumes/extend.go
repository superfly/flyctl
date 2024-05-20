package volumes

import (
	"context"
	"fmt"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newExtend() *cobra.Command {
	const (
		short = "Extend a volume to the specified size."

		long = short + ` Most Machines don't require a restart after extending a volume. Some older Machines get a message to manually restart the Machine to increase the size of the file system.`

		usage = "extend <volume id>"
	)

	cmd := command.New(usage, short, long, runExtend,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "size",
			Shorthand:   "s",
			Description: "Target volume size in gigabytes",
		},
		flag.Yes(),
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runExtend(ctx context.Context) error {
	var (
		cfg      = config.FromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		appName  = appconfig.NameFromContext(ctx)
		client   = flyutil.ClientFromContext(ctx)
		volID    = flag.FirstArg(ctx)
	)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	app, err := client.GetAppBasic(ctx, appName)
	if err != nil {
		return err
	}

	sizeFlag := flag.GetString(ctx, "size")
	sizeGB, err := helpers.ParseSize(sizeFlag, units.FromHumanSize, units.GB)
	if err != nil {
		return err
	}

	if sizeGB == 0 {
		return fmt.Errorf("Volume size must be specified")
	}

	if sizeFlag[0] == '+' {
		volume, err := flapsClient.GetVolume(ctx, volID)
		if err != nil {
			return err
		}
		sizeGB += volume.SizeGb
	}

	if volID == "" {
		volume, err := selectVolume(ctx, flapsClient, app)
		if err != nil {
			return err
		}
		volID = volume.ID
	}

	volume, needsRestart, err := flapsClient.ExtendVolume(ctx, volID, sizeGB)
	if err != nil {
		return fmt.Errorf("failed to extend volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	if err := printVolume(out, volume, appName); err != nil {
		return err
	}

	if needsRestart {
		fmt.Fprintln(out, colorize.Yellow("You will need to stop and start your Machine to increase the size of the file system"))
	} else {
		fmt.Fprintln(out, colorize.Green("Your Machine got its volume size extended without needing a restart"))
	}

	return nil
}
