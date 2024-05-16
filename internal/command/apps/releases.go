package apps

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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
		flag.JSONOutput(),
		flag.Bool{
			Name:        "image",
			Description: "Display the Docker image reference of the release",
		},
	)

	return
}

func runReleases(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
		out     = iostreams.FromContext(ctx).Out
	)

	releases, err := client.GetAppReleasesMachines(ctx, appName, "", 25)
	if err != nil {
		return fmt.Errorf("failed retrieving app releases %s: %w", appName, err)
	}

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Version > releases[j].Version
	})

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, releases)
	}

	rows, headers := formatMachinesReleases(releases, flag.GetBool(ctx, "image"))
	return render.Table(out, "", rows, headers...)
}

func formatMachinesReleases(releases []fly.Release, image bool) ([][]string, []string) {
	var rows [][]string
	for _, release := range releases {
		row := []string{
			fmt.Sprintf("v%d", release.Version),
			release.Status,
			release.Description,
			release.User.Email,
			format.RelativeTime(release.CreatedAt),
		}
		if image {
			row = append(row, release.ImageRef)
		}
		rows = append(rows, row)
	}

	headers := []string{
		"Version",
		"Status",
		"Description",
		"User",
		"Date",
	}
	if image {
		headers = append(headers, "Docker Image")
	}

	return rows, headers
}
