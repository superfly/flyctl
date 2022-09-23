package vm

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/logs"
)

func newStatus() *cobra.Command {
	const (
		short = "Show a VM's status"
		long  = short + "\t" + "including logs, checks, and events." + "\n"
		usage = "status <vm-id>"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		appName  = app.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
		logLimit = 25
	)

	// vm status is not supported for machines
	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to retrieve app: %w", err)
	}

	if app.PlatformVersion == "machines" {
		return fmt.Errorf("it looks like your app is running on v2 of our platform, and does not support this legacy command: try running fly machine status instead")
	}

	alloc, err := client.GetAllocationStatus(ctx, appName, flag.FirstArg(ctx), logLimit)
	if err != nil {
		return fmt.Errorf("failed to fetch allocation status: %w", err)
	}

	if err = render.AllocationStatus(io.Out, "Instance", alloc); err != nil {
		return
	}

	if err = render.AllocationEvents(io.Out, "Events", alloc.Events...); err != nil {
		return
	}

	if err = render.AllocationChecks(io.Out, "Checks", alloc.Checks...); err != nil {
		return
	}

	// render recent logs
	var entries []logs.LogEntry

	// convert alloc.RecentLogs to type logs.LogEntry and add them to entries
	for _, e := range alloc.RecentLogs {
		entries = append(entries, logs.LogEntry{
			Timestamp: e.Timestamp,
			Message:   e.Message,
			Level:     e.Level,
		})
	}

	if err = render.AllocationLogs(io.Out, "Recent Logs", entries); err != nil {
		return
	}

	return
}
