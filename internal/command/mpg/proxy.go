package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
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
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runProxy(ctx context.Context) (err error) {
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}

	localProxyPort := "16380"
	_, params, password, err := getMpgProxyParams(ctx, org.Slug, localProxyPort)
	if err != nil {
		return err
	}

	terminal.Infof("Proxying postgres to port \"%s\" with password \"%s\"", localProxyPort, password)

	return proxy.Connect(ctx, params)
}

func getMpgProxyParams(ctx context.Context, orgSlug string, localProxyPort string) (*uiex.ManagedCluster, *proxy.ConnectParams, string, error) {
	client := flyutil.ClientFromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	var index int
	var options []string

	clustersResponse, err := uiexClient.ListManagedClusters(ctx, orgSlug)
	if err != nil {
		return nil, nil, "", err
	}

	for _, cluster := range clustersResponse.Data {
		options = append(options, fmt.Sprintf("%s (%s)", cluster.Name, cluster.Region))
	}

	selectErr := prompt.Select(ctx, &index, "Select a database to connect to", "", options...)
	if selectErr != nil {
		return nil, nil, "", selectErr
	}

	selectedCluster := clustersResponse.Data[index]

	response, err := uiexClient.GetManagedCluster(ctx, selectedCluster.Organization.Slug, selectedCluster.Id)
	if err != nil {
		return nil, nil, "", err
	}
	cluster := response.Data

	if response.Password.Status == "initializing" {
		return nil, nil, "", fmt.Errorf("Cluster is still initializing, wait a bit more")
	}

	if response.Password.Status == "error" {
		return nil, nil, "", fmt.Errorf("Error getting cluster password")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, nil, "", err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, orgSlug, "", false)
	if err != nil {
		return nil, nil, "", err
	}

	return &cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "5432"},
		OrganizationSlug: orgSlug,
		Dialer:           dialer,
		RemoteHost:       cluster.IpAssignments.Direct,
	}, response.Password.Value, nil
}
