package resume

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `The RESUME command will restart a previously suspended application.
The application will resume with its original region pool and a min count of one
meaning there will be one running instance once restarted. Use SCALE SET MIN= to raise
the number of configured instances.
`
		short = "Resume an application"
		usage = "resume <APPNAME>"
	)

	resume := command.New(usage, short, long, apps.RunResume,
		command.RequireSession)
	resume.Hidden = true
	resume.Deprecated = "use `fly scale count` instead"

	resume.Args = cobra.ExactArgs(1)

	return resume
}
