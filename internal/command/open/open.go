package open

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command/apps"
)

// TODO: deprecate
func New() *cobra.Command {
	cmd := apps.NewOpen()
	cmd.Deprecated = "use `fly apps open` instead"
	cmd.Hidden = true
	return cmd
}
