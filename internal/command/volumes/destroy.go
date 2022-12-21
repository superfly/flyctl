package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

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
	cmd.Aliases = []string{"rm"}

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDestroy(ctx context.Context) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
		volID    = flag.FirstArg(ctx)
	)

	if !flag.GetYes(ctx) {
		const msg = "Deleting a volume is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirm(ctx, "Are you sure you want to destroy this volume?"); {
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
		return fmt.Errorf("failed destroying volume: %w", err)
	}

	fmt.Fprintf(io.Out, "Destroyed volume %s from %s\n", volID, data.Name)

	return nil
}
