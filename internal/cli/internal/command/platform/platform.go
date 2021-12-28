// Package platform implements the platform command chain.
package platform

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

// New initializes and returns a new apps Command.
func New() (cmd *cobra.Command) {
	const (
		long = `The PLATFORM commands are for users looking for information
about the Fly platform.
`
		short = "Fly platform information"
	)

	cmd = command.New("platform", short, long, nil)

	cmd.AddCommand(
		newRegions(),
		newStatus(),
		newVMSizes(),
	)

	return
}
