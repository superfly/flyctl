package apps

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newResume() *cobra.Command {
	resume := command.New("resume <APPNAME>", "", "", nil)
	resume.Hidden = true
	resume.Deprecated = "use `fly scale count` instead"
	return resume
}
