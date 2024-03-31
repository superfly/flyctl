package suspend

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	suspend := command.New("suspend [APPNAME]", "", "", nil, command.RequireSession)
	suspend.Hidden = true
	suspend.Deprecated = "use `fly scale count` instead"
	return suspend
}
