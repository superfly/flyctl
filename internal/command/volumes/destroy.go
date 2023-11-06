package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDestroy() *cobra.Command {
	const (
		short = "Destroy a volume."

		long = short + " When you destroy a volume, you permanently delete all its data."
	)

	cmd := command.New("destroy [id]", short, long, runDestroy,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"delete", "rm"}

	flag.Add(cmd,
		flag.Yes(),
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runDestroy(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
		volID  = flag.FirstArg(ctx)
	)

	appName := appconfig.NameFromContext(ctx)
	if volID == "" && appName == "" {
		return fmt.Errorf("volume ID or app required")
	}

	if appName == "" {
		n, err := client.GetAppNameFromVolume(ctx, volID)
		if err != nil {
			return err
		}
		appName = *n
	}

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	if volID == "" {
		app, err := client.GetApp(ctx, appName)
		if err != nil {
			return err
		}
		volume, err := selectVolume(ctx, flapsClient, app)
		if err != nil {
			return err
		}
		volID = volume.ID
	}

	if confirm, err := confirmVolumeDelete(ctx, volID); err != nil {
		return err
	} else if !confirm {
		return nil
	}

	data, err := flapsClient.DeleteVolume(ctx, volID)
	if err != nil {
		return fmt.Errorf("failed destroying volume: %w", err)
	}

	fmt.Fprintf(io.Out, "Destroyed volume ID: %s name: %s\n", volID, data.Name)

	return nil
}

func confirmVolumeDelete(ctx context.Context, volID string) (bool, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()

		err error
	)

	if flag.GetYes(ctx) {
		return true, nil
	}

	// fetch the volume so we can get the associated app
	var volume *api.Volume
	if volume, err = flapsClient.GetVolume(ctx, volID); err != nil {
		return false, err
	}

	// fetch the set of volumes for this app. If > 0 we skip the prompt
	var matches int32
	if matches, err = countVolumesMatchingName(ctx, volume.Name); err != nil {
		return false, err
	}

	var msg = "Deleting a volume is not reversible."
	if matches <= 2 {
		msg = fmt.Sprintf("Warning! Every volume is pinned to a specific physical host. You should create two or more volumes per application. Deleting this volume will leave you with %d volume(s) for this application, and it is not reversible.  Learn more at https://fly.io/docs/reference/volumes/", matches-1)
	}
	fmt.Fprintln(io.ErrOut, colorize.Red(msg))

	switch confirmed, err := prompt.Confirm(ctx, "Are you sure you want to destroy this volume?"); {
	case err == nil:
		return confirmed, nil
	case prompt.IsNonInteractive(err):
		return false, prompt.NonInteractiveError("yes flag must be specified when not running interactively")
	default:
		return false, err
	}
}
