package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newReset() (cmd *cobra.Command) {
	const (
		short = "Reset the password for an Upstash Redis database"
		long  = `Reset the password for the Upstash Redis database given by name as the
argument, and print the new connection URL. There's no confirmation
prompt. Update any app secrets or configuration that use the old URL.`

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
		client   = flyutil.ClientFromContext(ctx).GenqClient()
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
