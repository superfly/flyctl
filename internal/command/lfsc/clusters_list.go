package lfsc

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newClustersList() *cobra.Command {
	const (
		long = `Lists the LiteFS Cloud clusters in the organization.`

		short = "Show LiteFS Cloud clusters"

		usage = "list <clustername>"
	)

	cmd := command.New(usage, short, long, runClustersList,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		urlFlag(),
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runClustersList(ctx context.Context) error {
	cfg := config.FromContext(ctx)

	lfscClient, err := newLFSCClient(ctx, "")
	if err != nil {
		return err
	}

	output, err := lfscClient.ListClusters(ctx, nil)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
		_ = render.JSON(out, output)
		return nil
	}

	rows := make([][]string, 0, len(output.Clusters))
	for _, cluster := range output.Clusters {
		rows = append(rows, []string{
			cluster.Name,
			cluster.Region,
			format.RelativeTime(cluster.CreatedAt),
		})
	}

	_ = render.Table(out, "", rows, "Name", "Region", "Created")

	return nil
}
