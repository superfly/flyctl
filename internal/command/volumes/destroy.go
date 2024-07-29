package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newDestroy() *cobra.Command {
	const (
		short = "Destroy one or more volumes."

		long = short + " When you destroy a volume, you permanently delete all its data."
	)

	cmd := command.New("destroy <volume id> ... [flags]", short, long, runDestroy,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)
	cmd.Args = cobra.ArbitraryArgs
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
		client = flyutil.ClientFromContext(ctx)
		volIDs = flag.Args(ctx)
	)

	appName := appconfig.NameFromContext(ctx)
	if len(volIDs) == 0 && appName == "" {
		return fmt.Errorf("volume ID or app required")
	}

	if appName == "" {
		n, err := client.GetAppNameFromVolume(ctx, volIDs[0])
		if err != nil {
			return err
		}
		appName = *n
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	if len(volIDs) == 0 {
		app, err := client.GetAppBasic(ctx, appName)
		if err != nil {
			return err
		}
		volume, err := selectVolume(ctx, flapsClient, app)
		if err != nil {
			return err
		}
		volIDs = append(volIDs, volume.ID)
	}

	for _, volID := range volIDs {
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
	}

	return nil
}

func confirmVolumeDelete(ctx context.Context, volID string) (bool, error) {
	var (
		flapsClient = flapsutil.ClientFromContext(ctx)
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()

		err error
	)

	if flag.GetYes(ctx) {
		return true, nil
	}

	// fetch the volume so we can get the associated app
	var volume *fly.Volume
	if volume, err = flapsClient.GetVolume(ctx, volID); err != nil {
		return false, err
	}

	// fetch the set of volumes for this app. If > 0 we skip the prompt
	var matches int32
	if matches, err = countVolumesMatchingName(ctx, volume.Name); err != nil {
		return false, err
	}

	msg := "Deleting a volume is not reversible."
	if matches <= 2 {
		msg = fmt.Sprintf("Warning! Every volume is pinned to a specific physical host. You should create two or more volumes per application. Deleting this volume will leave you with %d volume(s) for this application, and it is not reversible.  Learn more at https://fly.io/docs/volumes/overview/", matches-1)
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
