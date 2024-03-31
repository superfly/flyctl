// Package history implements the history command chain.
package history

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func New() (cmd *cobra.Command) {
	const (
		long = `List the history of changes in the application. Includes autoscaling
events and their results.
`
		short = "List an app's change history"
	)

	cmd = command.New("history", short, long, nil,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Deprecated = "Use `flyctl apps releases` instead"
	cmd.Hidden = true

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return
}
