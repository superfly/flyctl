package machine

import (
	"context"
	"flag"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newKill() *cobra.Command {
	const (
		short = "Kill a machine"
		long  = short + "\n"

		usage = "kill"
	)

	cmd := command.New(usage, short, long, runMachineKill,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runMachineKill(ctx context.Context) (err error) {
	var (
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)
	for _, arg := range flag.Args() {
		input := api.KillMachineInput{
			AppID: appName,
			ID:    arg,
		}

		machine, err := client.KillMachine(ctx, input)
		if err != nil {
			return fmt.Errorf("could not stop machine %s: %w", arg, err)
		}

		fmt.Fprintf(out, "%s\n", machine.ID)
	}

	return
}
