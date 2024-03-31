package lfsc

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newClusters() *cobra.Command {
	const (
		long = `"Commands for managing LiteFS Cloud clusters"
`
		short = "Manage LiteFS Cloud clusters"
		usage = "clusters <command>"
	)

	cmd := command.New(usage, short, long, nil,
		command.RequireSession,
	)

	cmd.AddCommand(
		newClustersCreate(),
		newClustersDestroy(),
		newClustersList(),
	)

	return cmd
}
