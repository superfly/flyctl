package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newExtend() *cobra.Command {
	const (
		long = `Extends a target volume to the size specified. Volumes with attached nomad allocations 
		will be restarted automatically. Machines will require a manual restart to increase the size 
		of the FS.`

		short = "Extend a target volume"

		usage = "extend <id>"
	)

	cmd := command.New(usage, short, long, runExtend,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
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
		appName  = app.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
		volID    = flag.FirstArg(ctx)
	)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	sizeGB := flag.GetInt(ctx, "size")
	if sizeGB == 0 {
		return fmt.Errorf("Volume size must be specified")
	}

	if app.PlatformVersion == "nomad" {
		switch confirmed, err := prompt.Confirm(ctx, "Extending this volume will result in a VM restart. Continue?"); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	input := api.ExtendVolumeInput{
		VolumeID: volID,
		SizeGb:   flag.GetInt(ctx, "size"),
	}

	volume, err := client.ExtendVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to extend volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	if err := printVolume(out, volume); err != nil {
		return err
	}

	if app.PlatformVersion == "machines" {
		fmt.Fprintln(out, colorize.Yellow("You will need to stop and start your machine to increase the size of the FS"))
	}

	return nil
}
