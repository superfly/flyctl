package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
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
		short = "Destroy one or more volumes"

		long = `Destroy one or more Fly Volumes by ID, permanently deleting all their
data. This can't be undone. With no IDs, you pick a volume of the app from
a list in a terminal; without a terminal, IDs are required. Unless
` + "`--app`" + ` or ` + "`fly.toml`" + ` sets the app, it's looked up from the first volume ID.

fly asks you to confirm each volume. Pass ` + "`--yes`" + ` to skip the prompts;
it's required without a terminal. To keep a copy of the data, run
` + "`fly volumes fork`" + ` first.`
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

	flapsClient := flapsutil.ClientFromContext(ctx)

	if len(volIDs) == 0 {
		if _, err := flapsClient.GetApp(ctx, appName); err != nil {
			return err
		}
		volume, err := selectVolume(ctx, flapsClient, appName)
		if err != nil {
			return err
		}
		volIDs = append(volIDs, volume.ID)
	}

	for _, volID := range volIDs {
		if confirm, err := confirmVolumeDelete(ctx, appName, volID); err != nil {
			return err
		} else if !confirm {
			return nil
		}

		data, err := flapsClient.DeleteVolume(ctx, appName, volID)
		if err != nil {
			return fmt.Errorf("failed destroying volume: %w", err)
		}

		fmt.Fprintf(io.Out, "Destroyed volume ID: %s name: %s\n", volID, data.Name)
	}

	return nil
}

func confirmVolumeDelete(ctx context.Context, appName string, volID string) (bool, error) {
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
	if volume, err = flapsClient.GetVolume(ctx, appName, volID); err != nil {
		return false, err
	}

	// fetch the set of volumes for this app. If > 0 we skip the prompt
	var matches int32
	if matches, err = countVolumesMatchingName(ctx, appName, volume.Name); err != nil {
		return false, err
	}

	msg := "Deleting a volume is not reversible."
	if matches <= 2 {
		msg = fmt.Sprintf("Warning! Every volume is pinned to a specific physical host. You should create two or more volumes per application. Deleting this volume will leave you with %d volume(s) for this application, and it is not reversible.  Learn more at https://docs.fly.io/volumes/overview", matches-1)
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
