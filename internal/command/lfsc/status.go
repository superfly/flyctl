package lfsc

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() *cobra.Command {
	const (
		long = `Lists the databases in the cluster with their replication position.`

		short = "Show LiteFS Cloud cluster status"

		usage = "status"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		urlFlag(),
		clusterFlag(),
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runStatus(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	clusterName := flag.GetString(ctx, "cluster")
	if clusterName == "" {
		return errors.New("required: --cluster NAME")
	}

	lfscClient, err := newLFSCClient(ctx, clusterName)
	if err != nil {
		return err
	}

	posMap, err := lfscClient.Pos(ctx)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	if cfg.JSONOutput {
		_ = render.JSON(out, posMap)
		return nil
	}

	rows := make([][]string, 0, len(posMap))
	for name, pos := range posMap {
		rows = append(rows, []string{
			name,
			pos.TXID.String(),
			fmt.Sprintf("%016x", pos.PostApplyChecksum),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })

	_ = render.Table(out, "", rows, "Name", "TXID", "Checksum")

	return nil
}
