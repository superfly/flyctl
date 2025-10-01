package snapshots

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		long  = "List snapshots associated with the specified volume."
		short = "List snapshots."

		usage = "list <volume id>"
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
		client = flyutil.ClientFromContext(ctx)
	)

	volID := flag.FirstArg(ctx)

	appName := appconfig.NameFromContext(ctx)
	var volState string
	if appName == "" {
		n, s, err := client.GetAppNameStateFromVolume(ctx, volID)
		if err != nil {
			return fmt.Errorf("failed getting app name from volume: %w", err)
		}
		appName = *n
		volState = *s
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	var snapshots []fly.VolumeSnapshot
	switch volState {
	case "pending_destroy", "deleted":
		snapshots, err = client.GetSnapshotsFromVolume(ctx, volID)
	default:
		snapshots, err = flapsClient.GetVolumeSnapshots(ctx, volID)
	}
	if err != nil {
		return fmt.Errorf("failed retrieving snapshots: %w", err)
	}

	// sort snapshots from oldest to newest
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.Before(snapshots[j].CreatedAt)
	})

	if cfg.JSONOutput {
		return render.JSON(io.Out, snapshots)
	}

	if len(snapshots) == 0 {
		fmt.Fprintf(io.ErrOut, "No snapshots available for volume %s\n", volID)
		return nil
	}

	rows := make([][]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		id := snapshot.ID
		if id == "" {
			id = "(pending)"
		}

		retentionDays := ""
		if snapshot.RetentionDays != nil {
			retentionDays = strconv.Itoa(*snapshot.RetentionDays)
		}

		storedSize := humanize.IBytes(uint64(snapshot.Size))
		volSize := humanize.IBytes(uint64(snapshot.VolumeSize))

		rows = append(rows, []string{
			id,
			snapshot.Status,
			storedSize,
			volSize,
			timeToString(snapshot.CreatedAt),
			retentionDays,
		})
	}

	return render.Table(io.Out, "Snapshots", rows, "ID", "Status", "Stored Size", "Vol Size", "Created At", "Retention Days")
}
