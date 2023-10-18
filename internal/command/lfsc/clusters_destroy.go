package lfsc

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newClustersDestroy() *cobra.Command {
	const (
		long = `Permanently deletes a LiteFS Cloud cluster.`

		short = "Delete a LiteFS Cloud cluster"

		usage = "destroy"
	)

	cmd := command.New(usage, short, long, runClustersDestroy,
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

func runClustersDestroy(ctx context.Context) error {
	clusterName := flag.GetString(ctx, "cluster")
	if clusterName == "" {
		return errors.New("required: --cluster NAME")
	}

	lfscClient, err := newLFSCClient(ctx, "")
	if err != nil {
		return err
	}

	if err := lfscClient.DeleteCluster(ctx, clusterName); err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	fmt.Fprintf(out, "Cluster %q successfully deleted.\n", clusterName)

	return nil
}
