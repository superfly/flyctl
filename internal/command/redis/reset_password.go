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
)

func newReset() (cmd *cobra.Command) {
	const (
		long = `Reset the password for an Upstash Redis database`

		short = long
		usage = "reset <name>"
	)

	cmd = command.New(usage, short, long, runReset, command.RequireSession)

	flag.Add(cmd)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runReset(ctx context.Context) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		client   = client.FromContext(ctx).API().GenqClient
		colorize = io.ColorScheme()
		out      = io.Out
	)

	name := flag.FirstArg(ctx)

	_ = `# @genqlient
  mutation ResetAddOnPassword($name: String!) {
		resetAddOnPassword(input: {name: $name}) {
			addOn {
				publicUrl
			}
		}
  }
	`

	response, err := gql.ResetAddOnPassword(ctx, client, name)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "The password for your Redis database %s was reset.\n", name)
	fmt.Fprintf(out, "Your new Redis connection URL is %s\n", colorize.Green(response.ResetAddOnPassword.AddOn.PublicUrl))

	return
}
