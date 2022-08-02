package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newDelete() (cmd *cobra.Command) {
	const (
		long = `Delete a Redis instance`

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
		client = client.FromContext(ctx).API()
	)

	if err != nil {
		return err
	}

	id := flag.FirstArg(ctx)

	_, err = client.DeleteAddOn(ctx, id)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Redis instance %s was deleted\n", id)

	return
}
