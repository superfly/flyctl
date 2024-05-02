package arcjet

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Protect your applications with Arcjet"
		long  = short + "\n"
	)

	cmd = command.New("arcjet", short, long, nil)
	cmd.AddCommand(create())

	return cmd
}
