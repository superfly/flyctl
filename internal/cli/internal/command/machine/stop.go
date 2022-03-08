package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func newStop() *cobra.Command {
	const (
		short = "Stop a machine"
		long  = short + "\n"

		usage = "stop"
	)

	cmd := command.New(usage, short, long, runMachineStop,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(
		cmd,
		flag.String{
			Name:        "signal",
			Shorthand:   "s",
			Description: "Signal to stop the machine with (default: SIGINT)",
		},

		flag.String{
			Name:        "time",
			Description: "Seconds to wait before killing the machine",
		},
	)

	return cmd
}

func runMachineStop(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
