// Package history implements the history command chain.
package history

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
)

func New() (cmd *cobra.Command) {
	const (
		long = `List the history of changes in the application. Includes autoscaling
events and their results.
`
		short = "List an app's change history"
	)

	cmd = command.New("history", short, long, run,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func run(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()

	changes, err := client.GetAppChanges(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving history for %s: %w", appName, err)
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, changes)
	}

	var rows [][]string
	for _, change := range changes {
		rows = append(rows, []string{
			change.Actor.Type,
			change.Status,
			change.Description,
			change.User.Email,
			format.RelativeTime(change.CreatedAt),
		})
	}

	return render.Table(out, "", rows,
		"Type",
		"Status",
		"Description",
		"User",
		"Date",
	)
}
