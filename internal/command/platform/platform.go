// Package platform implements the platform command chain.
package platform

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new platform Command.
func New() (cmd *cobra.Command) {
	const (
		long = `Look up information about the Fly.io platform. ` + "`fly platform regions`" + ` lists
regions, ` + "`fly platform vm-sizes`" + ` lists Machine sizes, and
` + "`fly platform status`" + ` shows the platform's current status.`
		short = "Show Fly.io platform information"
	)

	cmd = command.New("platform", short, long, nil)

	cmd.AddCommand(
		newRegions(),
		newStatus(),
		newVMSizes(),
	)

	return
}
