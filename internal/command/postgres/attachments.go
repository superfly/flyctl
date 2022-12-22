package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newAttachments() *cobra.Command {
	const (
		short = "manage database attachments to apps"
		long  = short + "\n"
		usage = "attachments [command]"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newAttachmentsList(),
		newAttach(),
		newDetach(),
	)

	return cmd
}

func newAttachmentsList() *cobra.Command {
	const (
		short = "list database attachments"
		long  = short + "\n"
		usage = "list"
	)

	cmd := command.New(usage, short, long, runAttachmentsList,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	cmd.Aliases = []string{"ls"}

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runAttachmentsList(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		cfg     = config.FromContext(ctx)
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	attachments, err := client.ListPostgresClusterAttachments(ctx, app.ID)
	if err != nil {
		return fmt.Errorf("list attachments: %w", err)
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, attachments)
	}

	rows := [][]string{}

	for _, a := range attachments {
		rows = append(rows, []string{
			a.ID,
			a.DatabaseName,
			a.DatabaseUser,
			a.EnvironmentVariableName,
		})
	}

	var title = fmt.Sprintf("Attachments for %s", appName)

	render.Table(io.Out, title, rows, "ID", "Database Name", "Database User", "Environment Variable")

	return
}
