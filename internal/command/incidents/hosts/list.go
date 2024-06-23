package hosts

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
)

func list() (cmd *cobra.Command) {
	const (
		long  = `List hosts' issues affecting the application`
		short = long
		usage = "list"
	)

	cmd = command.New(usage, short, long, runList,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func runList(ctx context.Context) (err error) {
	var (
		out       = iostreams.FromContext(ctx).Out
		apiClient = flyutil.ClientFromContext(ctx)
		appName   = appconfig.NameFromContext(ctx)
	)

	appHostIssues, err := apiClient.GetAppHostIssues(ctx, appName)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, appHostIssues)
	}

	appHostIssuesCount := len(appHostIssues)
	if appHostIssuesCount > 0 {
		fmt.Fprintf(out, "Host Issues count: %d\n\n", appHostIssuesCount)
		table := helpers.MakeSimpleTable(out, []string{"Id", "Message", "Started At", "Last Updated"})
		table.SetRowLine(true)
		for _, appHostIssue := range appHostIssues {
			table.Append([]string{appHostIssue.InternalId, appHostIssue.Message, appHostIssue.CreatedAt.String(), appHostIssue.UpdatedAt.String()})
		}
		table.Render()
	} else {
		fmt.Fprintf(out, "There are no active host issues\n")
	}

	return nil
}
