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

func newDestroy() (cmd *cobra.Command) {
	const (
		long = `Permanently destroy an Upstash Redis database`

		short = long
		usage = "destroy <name>"
	)

	cmd = command.New(usage, short, long, runDestroy, command.RequireSession)

	cmd.Aliases = []string{"delete"}
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDestroy(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	appName := flag.FirstArg(ctx)

	if !flag.GetYes(ctx) {
		const msg = "Destroying a Redis instance is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Destroy Redis instance %s?", appName); {
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

	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	name := flag.FirstArg(ctx)

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

	fmt.Fprintf(out, "Your Redis database %s was destroyed\n", name)

	return
}
