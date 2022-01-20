package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
)

func newResolve() (cmd *cobra.Command) {
	const (
		short = "Resolve the address of a host"
		long  = short + "\n"
		usage = "resolve <slug> <app> <instance-id>"
	)

	cmd = command.New(usage, short, long, runResolve,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(3)

	return
}

func runResolve(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = establish(ctx); err != nil {
		return
	}

	args := flag.Args(ctx)
	host := fmt.Sprintf("%s.vm.%s.internal", args[2], args[1])

	addr, err := client.Resolve(ctx, flag.FirstArg(ctx), host)
	if err != nil {
		err = fmt.Errorf("failed resolving %s: %w", host, err)

		return
	}

	if out := iostreams.FromContext(ctx).Out; config.FromContext(ctx).JSONOutput {
		err = render.JSON(out, struct {
			Host string `json:"host"`
			Addr string `json:"addr"`
		}{
			Host: host,
			Addr: addr,
		})
	} else {
		_, err = fmt.Fprintf(out, "%s resolves to %s\n", host, addr)
	}

	return
}
