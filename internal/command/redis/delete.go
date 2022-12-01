package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDelete() (cmd *cobra.Command) {
	const (
		long = `Delete an Upstash Redis database`

		short = long
		usage = "delete <name>"
	)

	cmd = command.New(usage, short, long, runDelete, command.RequireSession)

	flag.Add(cmd)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runDelete(ctx context.Context) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		out      = io.Out
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API().GenqClient
	)

	name := flag.FirstArg(ctx)

	const msg = "Deleting a redis instance is not reversible."
	fmt.Fprintln(out, colorize.Red(msg))

	switch confirmed, err := prompt.Confirmf(ctx, "Destroy redis instance for app %s?", name); {
	case err == nil:
		if !confirmed {
			return nil
		}
	default:
		return err
	}

	_ = `# @genqlient
	  mutation DeleteAddOn($name: String) {
			deleteAddOn(input: {name: $name}) {
				deletedAddOnName
			}
	  }
		`

	_, err = gql.DeleteAddOn(ctx, client, name)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Redis database %s was deleted\n", name)

	return
}
