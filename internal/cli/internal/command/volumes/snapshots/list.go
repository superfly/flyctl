package snapshots

import (
	"context"
	"fmt"
	"sort"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
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

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runList(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	volID := flag.FirstArg(ctx)

	snapshots, err := client.GetVolumeSnapshots(ctx, volID)
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, snapshots)
	}

	if len(snapshots) == 0 {
		fmt.Fprintf(io.Out, "No snapshots available for volume %q\n", volID)
		return nil
	}

	// sort snapshots from newest to oldest
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	rows := make([][]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		rows = append(rows, []string{
			snapshot.ID,
			snapshot.Size,
			humanize.Time(snapshot.CreatedAt),
		})
	}

	return render.Table(io.Out, "Snapshots", rows, "ID", "Size", "Created At")
}
