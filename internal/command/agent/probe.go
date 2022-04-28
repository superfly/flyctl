package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newProbe() (cmd *cobra.Command) {
	const (
		short = "Probe tunnel for org"
		long  = short + "\n"
		usage = "probe <slug>"
	)

	cmd = command.New(usage, short, long, runProbe,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(1)

	return
}

func runProbe(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = establish(ctx); err != nil {
		return
	}

	if err = client.Probe(ctx, flag.FirstArg(ctx)); err != nil {
		return
	}

	out := iostreams.FromContext(ctx).Out
	_, err = fmt.Fprintln(out, "tunnel is up")

	return
}
