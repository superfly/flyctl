package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func newClone() *cobra.Command {
	const (
		short = "Clone a machine"
		long  = short + "\n"

		usage = "clone"
	)

	cmd := command.New(usage, short, long, runMachineClone,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(
		cmd,
		flag.String{
			Name:        "region",
			Shorthand:   "r",
			Description: "Target region",
		},
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the machine",
		},
		flag.String{
			Name:        "organization",
			Shorthand:   "o",
			Description: "Target organization",
		},
	)

	return cmd
}

func runMachineClone(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
