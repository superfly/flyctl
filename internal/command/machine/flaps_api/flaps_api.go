package flaps_api

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Commands to interact directly with the machines API"
		long  = short + "\n"
		usage = "flaps-api"
	)

	flaps_api := command.New(usage, short, long, nil,
		command.RequireSession,
	)

	flaps_api.Aliases = []string{"flaps"}

	flaps_api.AddCommand(
		newPost(),
	)

	return flaps_api
}
