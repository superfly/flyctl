package apps

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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
		flag.Bool{
			Name:        "image",
			Description: "Display the Docker image reference of the release",
		},
	)

	return
}

func runReleases(ctx context.Context) error {
	var (
		appName  = appconfig.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
		releases []api.Release
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if app.PlatformVersion == "machines" {
		releases, err = client.GetAppReleasesMachines(ctx, appName, 25)
	} else {
		releases, err = client.GetAppReleasesNomad(ctx, appName, 25)
	}

	if err != nil {
		return fmt.Errorf("failed retrieving app releases %s: %w", appName, err)
	}

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Version > releases[j].Version
	})

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, releases)
	}

	var (
		rows    [][]string
		headers []string
	)
	if app.PlatformVersion == "machines" {
		rows, headers = formatMachinesReleases(releases, flag.GetBool(ctx, "image"))
	} else {
		rows, headers = formatNomadReleases(releases, flag.GetBool(ctx, "image"))
	}
	return render.Table(out, "", rows, headers...)
}

func formatMachinesReleases(releases []api.Release, image bool) ([][]string, []string) {
	var rows [][]string
	for _, release := range releases {
		row := []string{
			fmt.Sprintf("v%d", release.Version),
			release.Status,
			release.Description,
			release.User.Email,
			presenters.FormatRelativeTime(release.CreatedAt),
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

func formatNomadReleases(releases []api.Release, image bool) ([][]string, []string) {
	var rows [][]string
	for _, release := range releases {
		row := []string{
			fmt.Sprintf("v%d", release.Version),
			fmt.Sprintf("%t", release.Stable),
			formatReleaseReason(release.Reason),
			release.Status,
			formatReleaseDescription(release),
			release.User.Email,
			presenters.FormatRelativeTime(release.CreatedAt),
		}
		if image {
			row = append(row, release.ImageRef)
		}
		rows = append(rows, row)
	}

	headers := []string{
		"Version",
		"Stable",
		"Type",
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
