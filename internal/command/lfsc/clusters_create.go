package lfsc

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newClustersCreate() *cobra.Command {
	const (
		long = `Creates a new LiteFS Cloud cluster.`

		short = "Creates a LiteFS Cloud cluster"

		usage = "create CLUSTERNAME"
	)

	cmd := command.New(usage, short, long, runClustersCreate,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(cmd,
		urlFlag(),
		regionFlag(),
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runClustersCreate(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()

	orgID, err := getOrgID(ctx)
	if err != nil {
		return err
	}

	clusterName := flag.FirstArg(ctx)
	if clusterName == "" {
		return errors.New("cluster name required as first argument")
	}
	region := flag.GetString(ctx, "region")
	if region == "" {
		return errors.New("required: --region CODE")
	}

	lfscClient, err := newLFSCClient(ctx, "")
	if err != nil {
		return err
	}

	cluster, err := lfscClient.CreateCluster(ctx, clusterName, region)
	if err != nil {
		return err
	}

	resp, err := gql.CreateLimitedAccessToken(ctx, apiClient.GenqClient, clusterName, orgID, "litefs_cloud",
		&gql.LimitedAccessTokenOptions{
			"cluster": clusterName,
		},
		"",
	)
	if err != nil {
		return fmt.Errorf("failed creating cluster token: %w", err)
	}

	out := iostreams.FromContext(ctx).Out
	fmt.Fprintf(out, "Cluster %q successfully created in %s.\n\n", cluster.Name, cluster.Region)
	fmt.Fprintf(out, "Run the following to set the auth token on your application:\n\n")
	fmt.Fprintf(out, "fly secrets set LFSC_AUTH_TOKEN=%q\n\n",
		resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader)

	return nil
}
