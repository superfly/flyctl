package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newExtend() *cobra.Command {
	const (
		long = ``

		short = "Extend a target volume"

		usage = "extend <id>"
	)

	cmd := command.New(usage, short, long, runExtend,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.Int{
			Name:        "size",
			Shorthand:   "s",
			Description: "Target volume size in gigabytes",
		},
	)

	return cmd
}

func runExtend(ctx context.Context) error {
	var (
		cfg      = config.FromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
		volID    = flag.FirstArg(ctx)
	)

	sizeGB := flag.GetInt(ctx, "size")
	if sizeGB == 0 {
		return fmt.Errorf("Volume size must be specified")
	}

	input := api.ExtendVolumeInput{
		VolumeID: volID,
		SizeGb:   flag.GetInt(ctx, "size"),
	}

	app, volume, err := client.ExtendVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to extend volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if app.PlatformVersion == "machines" {
		fmt.Fprintln(out, colorize.Yellow("You need to stop and start your machine to increase the size of the FS"))
	}

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	return printVolume(out, volume)
}
