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

func newStop() *cobra.Command {
	const (
		short = "Stop a Fly machine"
		long  = short + "\n"

		usage = "stop <id>"
	)

	cmd := command.New(usage, short, long, runMachineStop,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "signal",
			Shorthand:   "s",
			Description: "Signal to stop the machine with (default: SIGINT)",
		},

		flag.Int{
			Name:        "time",
			Description: "Seconds to wait before killing the machine",
		},
	)

	return cmd
}

func runMachineStop(ctx context.Context) (err error) {
	var (
		args    = flag.Args(ctx)
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)
	for _, arg := range args {
		input := api.StopMachineInput{
			AppID:           appName,
			ID:              arg,
			Signal:          flag.GetString(ctx, "signal"),
			KillTimeoutSecs: flag.GetInt(ctx, "time"),
		}

		machine, err := client.StopMachine(ctx, input)
		if err != nil {
			return fmt.Errorf("could not stop machine %s: %w", arg, err)
		}

		fmt.Fprintf(out, "%s\n", machine.ID)
	}
	return
}
