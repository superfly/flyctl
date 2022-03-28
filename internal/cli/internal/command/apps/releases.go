package apps

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/iostreams"
)

// TODO: make internal once the releases command has been deprecated
func NewReleases() (cmd *cobra.Command) {
	const (
		long = `List all the releases of the application onto the Fly platform,
including type, when, success/fail and which user triggered the release.
`
		short = "List app releases"
	)

	cmd = command.New("releases", short, long, runReleases,
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

func runReleases(ctx context.Context) error {
	appName := app.NameFromContext(ctx)

	releases, err := client.FromContext(ctx).API().GetAppReleases(ctx, appName, 25)
	if err != nil {
		return fmt.Errorf("failed retrieving app releases %s: %w", appName, err)
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, releases)
	}

	var rows [][]string

	for _, release := range releases {
		rows = append(rows, []string{
			fmt.Sprintf("v%d", release.Version),
			fmt.Sprintf("%t", release.Stable),
			formatReleaseReason(release.Reason),
			release.Status,
			formatReleaseDescription(release),
			release.User.Email,
			presenters.FormatRelativeTime(release.CreatedAt),
		})
	}

	return render.Table(out, "", rows,
		"Version",
		"Stable",
		"Type",
		"Status",
		"Description",
		"User",
		"Date",
	)
}

func formatReleaseReason(reason string) string {
	switch reason {
	case "change_image":
		return "Image"
	case "change_secrets":
		return "Secrets"
	case "change_code", "change_source": // nodeproxy
		return "Code Change"
	}
	return reason
}

func formatReleaseDescription(r api.Release) string {
	if r.Reason == "change_image" && strings.HasPrefix(r.Description, "deploy image ") {
		return r.Description[13:]
	}
	return r.Description
}
