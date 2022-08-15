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

func newDelete() (cmd *cobra.Command) {
	const (
		long = `Delete a Redis database`

		short = long
		usage = "delete [ID]"
	)

	cmd = command.New(usage, short, long, runDelete, command.RequireSession)

	flag.Add(cmd)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runDelete(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	id := flag.FirstArg(ctx)

	_ = `# @genqlient
  mutation DeleteAddOn($addOnId: ID!) {
		deleteAddOn(input: {addOnId: $addOnId}) {
			deletedAddOnId
		}
  }
	`

	_, err = gql.DeleteAddOn(ctx, client, id)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Redis database %s was deleted\n", id)

	return
}
