// Package status implements the status command chain.
package status

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/azazeal/pause"
	"github.com/inancgumus/screen"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Show the application's current status including application
details, tasks, most recent deployment details and in which regions it is
currently allocated.
`
		short = "Show app status"
	)

	cmd = command.New("status", short, long, run,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "all",
			Description: "Show completed instances",
		},
		flag.Bool{
			Name:        "deployment",
			Description: "Always show deployment status",
		},
		flag.Bool{
			Name:        "watch",
			Description: "Refresh details",
		},
		flag.Int{
			Name:        "rate",
			Description: "Refresh Rate for --watch",
			Default:     5,
		},
	)

	cmd.AddCommand(
		newInstance(),
	)

	return
}

func run(ctx context.Context) error {
	watch := flag.GetBool(ctx, "watch")
	if watch && config.FromContext(ctx).JSONOutput {
		return errors.New("--watch and --json are not supported together")
	}

	if !watch {
		return runOnce(ctx)
	}

	return runWatch(ctx)
}

func runOnce(ctx context.Context) error {
	return once(ctx, iostreams.FromContext(ctx).Out)
}

func once(ctx context.Context, out io.Writer) (err error) {
	var (
		appName    = app.NameFromContext(ctx)
		all        = flag.GetBool(ctx, "all")
		client     = client.FromContext(ctx).API()
		jsonOutput = config.FromContext(ctx).JSONOutput
	)

	var app *api.AppStatus
	if app, err = client.GetAppStatus(ctx, appName, all); err != nil {
		err = fmt.Errorf("failed retrieving app %s: %w", appName, err)

		return
	}

	var backupRegions []api.Region
	if app.Deployed && !jsonOutput {
		if _, backupRegions, err = client.ListAppRegions(ctx, appName); err != nil {
			return fmt.Errorf("failed retrieving backup regions for %s: %w", appName, err)
		}
	}

	if jsonOutput {
		err = render.JSON(out, app)

		return
	}

	obj := [][]string{
		{
			app.Name,
			app.Organization.Slug,
			strconv.Itoa(app.Version),
			app.Status,
			app.Hostname,
		},
	}

	if err = render.VerticalTable(out, "App", obj, "Name", "Owner", "Version", "Status", "Hostname"); err != nil {
		return
	}

	if !app.Deployed {
		_, err = fmt.Fprintln(out, "App has not been deployed yet.")

		return
	}

	showDeploymentStatus := app.DeploymentStatus != nil &&
		((app.DeploymentStatus.Version == app.Version && app.DeploymentStatus.Status != "cancelled") || flag.GetBool(ctx, "deployment"))

	if showDeploymentStatus {
		if err = renderDeploymentStatus(out, app.DeploymentStatus); err != nil {
			return
		}
	}

	err = render.AllocationStatuses(out, "Instances", backupRegions, app.Allocations...)

	return
}

func renderDeploymentStatus(w io.Writer, ds *api.DeploymentStatus) error {
	obj := [][]string{
		{
			ds.ID,
			fmt.Sprintf("v%d", ds.Version),
			ds.Status,
			ds.Description,
			fmt.Sprintf("%d desired, %d placed, %d healthy, %d unhealthy",
				ds.DesiredCount, ds.PlacedCount, ds.HealthyCount, ds.UnhealthyCount),
		},
	}

	return render.VerticalTable(w, "Deployment Status", obj,
		"ID",
		"Version",
		"Status",
		"Description",
		"Instances",
	)
}

func runWatch(ctx context.Context) (err error) {
	streams := iostreams.FromContext(ctx)
	if !streams.IsInteractive() {
		err = errors.New("--watch is not supported for non-interactive sessions")

		return
	}

	sleep := flag.GetInt(ctx, "rate")
	if sleep < 1 || sleep > 3600 {
		err = errors.New("--rate must be in the [1, 3600] range")

		return
	}

	appName := app.NameFromContext(ctx)

	var buf bytes.Buffer

	for err == nil {
		buf.Reset()

		if err = once(ctx, &buf); err != nil {
			break
		}

		header := fmt.Sprintf("%s %s %s\n\n", aurora.Bold(appName), aurora.Italic("at:"), aurora.Bold(time.Now().UTC().Format("15:04:05")))

		screen.Clear()
		screen.MoveTopLeft()

		io.Copy(streams.Out, io.MultiReader(
			strings.NewReader(header),
			&buf,
		))

		pause.For(ctx, time.Duration(sleep)*time.Second)
	}

	return
}
