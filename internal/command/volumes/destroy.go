package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDestroy() *cobra.Command {
	const (
		long = `Destroy a volume Requires the volume's ID
number to operate. This can be found through the volumes list command`

		short = "Destroy a volume"
	)

	cmd := command.New("destroy <id>", short, long, runDestroy,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"delete", "rm"}

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDestroy(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
		volID  = flag.FirstArg(ctx)
	)

	if confirm, err := confirmVolumeDelete(ctx, volID); err != nil {
		return err
	} else if !confirm {
		return nil
	}

	data, err := client.DeleteVolume(ctx, volID)
	if err != nil {
		return fmt.Errorf("failed destroying volume: %w", err)
	}

	fmt.Fprintf(io.Out, "Destroyed volume %s from %s\n", volID, data.Name)

	return nil
}

func confirmVolumeDelete(ctx context.Context, volID string) (bool, error) {
	var (
		client   = client.FromContext(ctx).API()
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()

		err error
	)

	if flag.GetYes(ctx) {
		return true, nil
	}

	// fetch the volume so we can get the associated app
	var volume *api.Volume
	if volume, err = client.GetVolume(ctx, volID); err != nil {
		return false, err
	}

	// fetch the set of volumes for this app. If > 0 we skip the prompt
	var matches int32
	if matches, err = countVolumesMatchingName(ctx, volume.App.Name, volume.Name); err != nil {
		return false, err
	}

	var msg = "Deleting a volume is not reversible."
	if matches <= 2 {
		msg = fmt.Sprintf("Warning! Individual volumes are pinned to individual hosts. You should create two or more volumes per application. Deleting this volume will leave you with %d volume(s) for this application, and it is not reversible.  Learn more at https://fly.io/docs/reference/volumes/", matches-1)
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
