package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newConnect() *cobra.Command {
	const (
		short = "Connect to the Postgres console"
		long  = short + "\n"
	)

	cmd := command.New("connect", short, long, nil)

	cmd.AddCommand()

	return cmd
}
