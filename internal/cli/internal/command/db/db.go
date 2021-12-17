// Package db implements the db command chain.
package db

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/db/redis"
)

// New initializes and returns a new db Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Create databases"
		long  = short + "\n"
	)

	cmd = command.New("db", short, long, nil)

	cmd.AddCommand(
		redis.New(),
	)

	return
}
