package volumes

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newDelete() *cobra.Command {
	const (
		long = `Delete a volume from the application. Requires the volume's ID
number to operate. This can be found through the volumes list command`

		short = "Delete a volume from the app"
	)

	cmd := command.New("delete <id>", short, long, runDelete)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDelete(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()

		volID = flag.FirstArg(ctx)
	)

	if !flag.GetYes(ctx) {

		fmt.Fprintln(io.Out, aurora.Red("Deleting a volume is not reversible."))

		if confirmed, err := prompt.Confirm(ctx, "Are you sure you want to delete this volume?"); err != nil || !confirmed {
			return err
		}
	}

	data, err := client.DeleteVolume(ctx, volID)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Deleted volume %s from %s\n", volID, data.Name)

	return nil
}
