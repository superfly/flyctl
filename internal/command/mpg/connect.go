package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/mpg/utils"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
)

const (
	localProxyPort = "16380"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a MPG database using psql`

		short = long
		usage = "connect <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)

	flag.Add(cmd,
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to connect as",
		},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	var cluster *utils.ManagedCluster

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, _, err = utils.ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
	}

	if cluster.Version == utils.V1 {
		return cmdv1.RunConnect(ctx, cluster.Id, cluster.Organization.ID, localProxyPort)
	}
	return cmdv2.RunConnect(ctx, clusterID, cluster.Organization.ID, localProxyPort)
}
