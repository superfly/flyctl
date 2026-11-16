package redis

import (
	"context"
	"fmt"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
		client   = flyutil.ClientFromContext(ctx).GenqClient()
		colorize = io.ColorScheme()
		out      = io.Out
	)

	name := flag.FirstArg(ctx)

	response, err := resetPassword(ctx, client, name)
	if err != nil {
		return
	}

	fmt.Fprintf(out, "The password for your Redis database %s was reset.\n", name)
	fmt.Fprintf(out, "Your new Redis connection URL is %s\n", colorize.Green(response.ResetAddOnPassword.AddOn.PublicUrl))

	return
}

// resetPassword names the provider, as destroy and status do, so the reset
// can't land on another kind of add-on with the same name.
func resetPassword(ctx context.Context, client genq.Client, name string) (*gql.ResetAddOnPasswordResponse, error) {
	_ = `# @genqlient
  mutation ResetAddOnPassword($name: String!, $provider: String) {
		resetAddOnPassword(input: {name: $name, provider: $provider}) {
			addOn {
				publicUrl
			}
		}
  }
	`

	return gql.ResetAddOnPassword(ctx, client, name, redisProvider)
}
