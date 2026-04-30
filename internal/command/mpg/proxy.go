package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/uiex/mpg"
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

func runProxy(ctx context.Context) (err error) {
	clusterID := flag.FirstArg(ctx)
	cluster, orgSlug, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	localProxyPort := flag.GetString(ctx, flagnames.LocalPort)

	// Resolve org slug alias for wireguard tunnel
	resolvedOrgSlug, err := AliasedOrganizationSlug(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("failed to resolve organization slug: %w", err)
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunProxy(ctx, cluster.Id, resolvedOrgSlug, localProxyPort)
	}

	return cmdv2.RunProxy(ctx, cluster.Id, resolvedOrgSlug, localProxyPort)
}
