package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDelete() *cobra.Command {
	const (
		long = `Delete a volume Requires the volume's ID
number to operate. This can be found through the volumes list command`

		short = "Delete a volume from the app"
	)

	cmd := command.New("delete <id>", short, long, runDelete,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDelete(ctx context.Context) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
		volID    = flag.FirstArg(ctx)
	)

	if !flag.GetYes(ctx) {
		const msg = "Deleting a volume is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirm(ctx, "Are you sure you want to delete this volume?"); {
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

	data, err := client.DeleteVolume(ctx, volID)
	if err != nil {
		return fmt.Errorf("failed deleting volume: %w", err)
	}

	fmt.Fprintf(io.Out, "Deleted volume %s from %s\n", volID, data.Name)

	return nil
}
