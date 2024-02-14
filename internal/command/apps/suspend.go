package apps

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newSuspend() *cobra.Command {
	suspend := command.New("suspend <APPNAME>", "", "", nil)
	suspend.Hidden = true
	suspend.Deprecated = "use `fly scale count` instead"
	return suspend
}
