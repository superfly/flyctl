package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newResume() *cobra.Command {
	const (
		long = `The APPS RESUME command will restart a previously suspended application. 
The application will resume with its original region pool and a min count of one
meaning there will be one running instance once restarted. Use SCALE SET MIN= to raise
the number of configured instances.
`

		short = "Resume an application"

		usage = "resume [APPNAME]"
	)

	resume := command.New(usage, short, long, runResume,
		command.RequireSession)

	resume.Args = cobra.RangeArgs(0, 1)

	return resume
}

func runResume(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
