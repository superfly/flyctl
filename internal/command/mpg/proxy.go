package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/proxy"
	"github.com/superfly/flyctl/terminal"
)

func newProxy() (cmd *cobra.Command) {
	const (
		long = `Proxy to a MPG database`

		short = long
		usage = "proxy"
	)

	cmd = command.New(usage, short, long, runProxy, command.RequireSession, command.RequireUiex)

	flag.Add(cmd,
		flag.Region(),
		flag.MPGCluster(),

		flag.String{
			Name:        flagnames.BindAddr,
			Shorthand:   "b",
			Default:     "127.0.0.1",
			Description: "Local address to bind to",
		},
	)

	return cmd
}

func runProxy(ctx context.Context) (err error) {
	localProxyPort := "16380"
	cluster, params, password, err := getMpgProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	name := fmt.Sprintf("pgdb-%s", cluster.Id)

	terminal.Infof("Proxying postgres to port \"%s\" with user \"%s\" password \"%s\"", localProxyPort, name, password)

	return proxy.Connect(ctx, params)
}

func getMpgProxyParams(ctx context.Context, localProxyPort string) (*uiex.ManagedCluster, *proxy.ConnectParams, string, error) {
	client := flyutil.ClientFromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	// Get cluster ID from flag or prompt user to select
	clusterID := flag.GetMPGClusterID(ctx)
	if clusterID == "" {
		return nil, nil, "", fmt.Errorf("cluster ID is required. Use --cluster flag or implement cluster selection")
	}

	// Get cluster details first to determine the organization
	response, err := uiexClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}
	cluster := response.Data

	// Get the organization from the cluster
	orgSlug := cluster.Organization.Slug

	if response.Credentials.Status == "initializing" {
		return nil, nil, "", fmt.Errorf("Cluster is still initializing, wait a bit more")
	}

	if response.Credentials.Status == "error" || response.Credentials.Password == "" {
		return nil, nil, "", fmt.Errorf("Error getting cluster password")
	}

	if cluster.IpAssignments.Direct == "" {
		return nil, nil, "", fmt.Errorf("Error getting cluster IP")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, nil, "", err
	}

	// Use the organization slug from the cluster for wireguard tunnel
	dialer, err := agentclient.ConnectToTunnel(ctx, orgSlug, "", false)
	if err != nil {
		return nil, nil, "", err
	}

	return &cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "5432"},
		OrganizationSlug: orgSlug,
		Dialer:           dialer,
		BindAddr:         flag.GetBindAddr(ctx),
		RemoteHost:       cluster.IpAssignments.Direct,
	}, response.Credentials.Password, nil
}
