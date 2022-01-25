package agent

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
)

func newEstablish() (cmd *cobra.Command) {
	const (
		short = "Establish"
		long  = short + "\n"
		usage = "establish <slug>"
	)

	cmd = command.New(usage, short, long, runEstablish)

	cmd.Args = cobra.ExactArgs(1)

	return
}

func runEstablish(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = establish(ctx); err != nil {
		return
	}

	var res *agent.EstablishResponse
	if res, err = client.Establish(ctx, flag.FirstArg(ctx)); err == nil {
		out := iostreams.FromContext(ctx).Out
		err = render.JSON(out, res)
	}

	return
}
