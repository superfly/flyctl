package status

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/format"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newInstance() (cmd *cobra.Command) {
	const (
		long = `Show the instance's current status including logs, checks,
and events.
`
		short = "Show instance status"
		usage = "instance <INSTANCE_ID>"
	)

	cmd = command.New(usage, short, long, runInstance,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runInstance(ctx context.Context) (err error) {
	var (
		appName = app.NameFromContext(ctx)
		allocID = flag.FirstArg(ctx)
		client  = client.FromContext(ctx).API()
	)

	var alloc *api.AllocationStatus
	if alloc, err = client.GetAllocationStatus(ctx, appName, allocID, 25); err != nil {
		err = fmt.Errorf("failed retrieving allocation status for %s: %w", allocID, err)

		return
	} else if alloc == nil {
		err = fmt.Errorf("could not find allocation with ID %s for app %s", allocID, appName)

		return
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		// TODO: checks & recent events are being outputted twice
		err = render.JSON(out,
			map[string]interface{}{
				"Instance":      alloc,
				"Recent Events": alloc.Events,
				"Checks":        alloc.Checks,
			},
		)

		return
	}

	var buf bytes.Buffer

	if err = renderAllocationStatus(&buf, alloc); err != nil {
		err = fmt.Errorf("failed rendering allocation status: %w", err)

		return
	}

	if err = render.AllocationEvents(&buf, "Recent Events", alloc.Events...); err != nil {
		err = fmt.Errorf("failed rendering allocation events: %w", err)

		return
	}

	if err = renderChecks(&buf, alloc.Checks); err != nil {
		err = fmt.Errorf("failed rendering checks: %w", err)

		return
	}

	_, err = buf.WriteTo(out)
	return
}

func renderAllocationStatus(w io.Writer, as *api.AllocationStatus) error {
	obj := [][]string{
		{
			as.IDShort,
			as.TaskName,
			strconv.Itoa(as.Version),
			as.Region,
			as.DesiredStatus,
			format.AllocStatus(as),
			format.HealthChecksSummary(as),
			strconv.Itoa(as.Restarts),
			format.RelativeTime(as.CreatedAt),
		},
	}

	return render.VerticalTable(w, "Instance", obj,
		"ID",
		"Process",
		"Version",
		"Region",
		"Desired",
		"Status",
		"Health Checks",
		"Restarts",
		"Created",
	)
}

func renderChecks(w io.Writer, checks []api.CheckState) error {
	var rows [][]string

	for _, check := range checks {
		rows = append(rows, []string{
			check.Name,
			check.ServiceName,
			check.Status,
			check.Output,
		})
	}

	return render.Table(w, "Checks", rows,
		"ID",
		"Service",
		"State",
		"Output",
	)
}
