package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newStart() *cobra.Command {
	const (
		short = "Start a Fly machine"
		long  = short + "\n"

		usage = "start <id>"
	)

	cmd := command.New(usage, short, long, runMachineStart,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runMachineStart(ctx context.Context) (err error) {
	var (
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
		id      = flag.FirstArg(ctx)
		client  = client.FromContext(ctx).API()
	)

	input := api.StartMachineInput{
		AppID: appName,
		ID:    id,
	}

	machine, err := client.StartMachine(ctx, input)
	if err != nil {
		return fmt.Errorf("could not start machine %s: %w", id, err)
	}

	fmt.Fprintf(out, "%s\n", machine.ID)

	return
}
