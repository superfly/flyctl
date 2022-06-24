package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newDetach() *cobra.Command {
	const (
		short = "Detach a postgres cluster from an app"
		long  = short + "\n"
	)

	cmd := command.New("detach", short, long, nil)

	return cmd
}
