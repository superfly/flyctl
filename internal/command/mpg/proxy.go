package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/mpg/utils"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

func newProxy() (cmd *cobra.Command) {
	const (
		long = `Proxy to a MPG database`

		short = long
		usage = "proxy <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runProxy, command.RequireSession)

	flag.Add(cmd,
		flag.String{
			Name:        flagnames.BindAddr,
			Shorthand:   "b",
			Default:     "127.0.0.1",
			Description: "Local address to bind to",
		},
		flag.String{
			Name:        flagnames.LocalPort,
			Shorthand:   "p",
			Default:     "16380",
			Description: "Local port to proxy on",
		},
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runProxy(ctx context.Context) error {
	var cluster *utils.ManagedCluster
	var orgSlug string
	var err error

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		cluster, orgSlug, err = utils.ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
	}

	localProxyPort := flag.GetString(ctx, flagnames.LocalPort)

	if cluster.Version == utils.V1 {
		return cmdv1.RunProxy(ctx, clusterID, localProxyPort, orgSlug)
	}

	return cmdv2.RunProxy(ctx, clusterID, localProxyPort, orgSlug)
}
