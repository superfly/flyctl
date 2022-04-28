// Package services implements the services command chain.
package services

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/services/postgres"
	"github.com/superfly/flyctl/internal/command/services/redis"
)

// New initializes and returns a new services Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Launch and manage services"
		long  = short + "\n"
	)

	cmd = command.New("services", short, long, nil)

	cmd.AddCommand(
		redis.New(),
		postgres.New(),
	)

	return
}
