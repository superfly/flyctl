package resume

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	resume := command.New("resume <APPNAME>", "", "", nil, command.RequireSession)
	resume.Hidden = true
	resume.Deprecated = "use `fly scale count` instead"
	return resume
}
