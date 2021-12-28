package open

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command/apps"
)

// TODO: deprecate
func New() *cobra.Command {
	return apps.NewOpen()
}
