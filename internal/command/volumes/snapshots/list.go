package snapshots

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		long  = "List snapshots associated with the specified volume"
		short = "List snapshots"

		usage = "list <volume-id>"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
	)

	cmd.Aliases = []string{"ls"}

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func timeToString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return humanize.Time(t)
}

func runList(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	volID := flag.FirstArg(ctx)

	appName := appconfig.NameFromContext(ctx)
	if appName == "" {
		n, err := client.GetAppNameFromVolume(ctx, volID)
		if err != nil {
			return err
		}
		appName = *n
	}

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	snapshots, err := flapsClient.GetVolumeSnapshots(ctx, volID)
	if err != nil {
		return fmt.Errorf("failed retrieving snapshots: %w", err)
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, snapshots)
	}

	if len(snapshots) == 0 {
		fmt.Fprintf(io.ErrOut, "No snapshots available for volume %s\n", volID)
		return nil
	}

	// sort snapshots from newest to oldest
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	rows := make([][]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		id := snapshot.ID
		if id == "" {
			id = "(pending)"
		}

		rows = append(rows, []string{
			id,
			snapshot.Status,
			strconv.Itoa(snapshot.Size),
			timeToString(snapshot.CreatedAt),
		})
	}

	return render.Table(io.Out, "Snapshots", rows, "ID", "Status", "Size", "Created At")
}
