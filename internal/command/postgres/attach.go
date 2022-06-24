package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newAttach() *cobra.Command {
	const (
		short = "Attach a postgres cluster to an app"
		long  = short + "\n"
	)

	cmd := command.New("attach", short, long, nil)

	return cmd
}
