package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newExtend() *cobra.Command {
	const (
		long = `Extends a target volume to the size specified. The instance is automatically restarted for Nomad (V1) apps.
		Most Machines (V2 apps) don't require a restart. Older Machines get a message to manually restart the Machine
		to increase the size of the FS.`

		short = "Extend a target volume"

		usage = "extend <id>"
	)

	cmd := command.New(usage, short, long, runExtend,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{
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
		client   = client.FromContext(ctx).API()
		volID    = flag.FirstArg(ctx)
	)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	sizeGB := flag.GetInt(ctx, "size")
	if sizeGB == 0 {
		return fmt.Errorf("Volume size must be specified")
	}

	if app.PlatformVersion == "nomad" {
		if !flag.GetYes(ctx) {
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
	}

	if volID == "" {
		volumes, err := flapsClient.GetVolumes(ctx)
		if err != nil {
			return err
		}
		volume, err := selectVolume(ctx, volumes, app)
		if err != nil {
			return err
		}
		volID = volume.ID
	}

	volume, needsRestart, err := flapsClient.ExtendVolume(ctx, volID, flag.GetInt(ctx, "size"))
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

	if app.PlatformVersion == "machines" {
		if needsRestart {
			fmt.Fprintln(out, colorize.Yellow("You will need to stop and start your machine to increase the size of the FS"))
		} else {
			fmt.Fprintln(out, colorize.Green("Your machine got its volume size extended without needing a restart"))
		}
	}

	return nil
}
