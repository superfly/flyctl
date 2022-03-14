package machine

import (
	"context"

	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newRemove() *cobra.Command {
	const (
		short = "Remove a Fly machine"
		long  = short + "\n"

		usage = "remove"
	)

	cmd := command.New(usage, short, long, runMachineRemove,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Aliases = []string{"rm"}

	flag.Add(
		cmd,
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "force kill machine if it's running",
		},
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runMachineRemove(ctx context.Context) (err error) {
	var (
		client = client.FromContext(ctx).API()
		out    = iostreams.FromContext(ctx).Out
	)
	for _, arg := range flag.Args(ctx) {
		input := api.RemoveMachineInput{
			AppID: app.NameFromContext(ctx),
			ID:    arg,
			Kill:  flag.GetBool(ctx, "force"),
		}

		machine, err := client.RemoveMachine(ctx, input)
		if err != nil {
			return fmt.Errorf("could not stop machine: %w", err)
		}

		fmt.Fprintln(out, machine.ID)
	}
	return
}
