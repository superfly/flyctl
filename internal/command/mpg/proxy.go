package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
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
	// This `org.Slug` could be "personal" and we need that for wireguard connections
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return nil, nil, "", err
	}

	client := flyutil.ClientFromContext(ctx)
	genqClient := flyutil.ClientFromContext(ctx).GenqClient()

	// For ui-ex request we need the real org slug
	var fullOrg *gql.GetOrganizationResponse
	if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
		err = fmt.Errorf("failed fetching org: %w", err)
		return nil, nil, "", err
	}

	uiexClient := uiexutil.ClientFromContext(ctx)

	var index int
	var options []string

	clustersResponse, err := uiexClient.ListManagedClusters(ctx, fullOrg.Organization.RawSlug)
	if err != nil {
		return nil, nil, "", err
	}

	// fmt.Printf("%+v\n", clustersResponse)
	// fmt.Printf("%+v\n", err)

	if len(clustersResponse.Data) == 0 {
		err := fmt.Errorf("No Managed Postgres clusters found on this organization")
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

	dialer, err := agentclient.ConnectToTunnel(ctx, org.Slug, "", false)
	if err != nil {
		return nil, nil, "", err
	}

	return &cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "5432"},
		OrganizationSlug: org.Slug,
		Dialer:           dialer,
		RemoteHost:       cluster.IpAssignments.Direct,
	}, response.Password.Value, nil
}
