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
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/proxy"
)

func newProxy() (cmd *cobra.Command) {
	const (
		long = `Proxy to a MPG database`

		short = long
		usage = "proxy <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runProxy, command.RequireSession, command.RequireUiex)

	flag.Add(cmd,
		flag.String{
			Name:        flagnames.BindAddr,
			Shorthand:   "b",
			Default:     "127.0.0.1",
			Description: "Local address to bind to",
		},
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runProxy(ctx context.Context) (err error) {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	localProxyPort := "16380"
	_, params, _, err := getMpgProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	return proxy.Connect(ctx, params)
}

func getMpgProxyParams(ctx context.Context, localProxyPort string) (*uiex.ManagedCluster, *proxy.ConnectParams, *uiex.GetManagedClusterCredentialsResponse, error) {
	client := flyutil.ClientFromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	// Get cluster ID from flag - it's optional now
	clusterID := flag.FirstArg(ctx)

	var cluster *uiex.ManagedCluster
	var orgSlug string
	var err error

	if clusterID != "" {
		// If cluster ID is provided, get cluster details directly and extract org info from it
		response, err := uiexClient.GetManagedClusterById(ctx, clusterID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
		}
		cluster = &response.Data
		orgSlug = cluster.Organization.Slug
	} else {
		// If no cluster ID is provided, let user select org first, then cluster
		org, err := orgs.OrgFromFlagOrSelect(ctx)
		if err != nil {
			return nil, nil, nil, err
		}

		// For ui-ex requests we need the real org slug (resolve aliases like "personal")
		genqClient := client.GenqClient()
		var fullOrg *gql.GetOrganizationResponse
		if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
			return nil, nil, nil, fmt.Errorf("failed fetching org: %w", err)
		}

		// Now let user select a cluster from this organization
		selectedCluster, err := ClusterFromArgOrSelect(ctx, clusterID, fullOrg.Organization.RawSlug)
		if err != nil {
			return nil, nil, nil, err
		}

		cluster = selectedCluster
		orgSlug = cluster.Organization.Slug
	}

	// At this point we have both cluster and orgSlug
	// Get credentials for the cluster
	response, err := uiexClient.GetManagedClusterById(ctx, cluster.Id)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed retrieving cluster credentials %s: %w", cluster.Id, err)
	}

	// Resolve organization slug to handle aliases
	resolvedOrgSlug, err := AliasedOrganizationSlug(ctx, orgSlug)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve organization slug: %w", err)
	}

	if response.Credentials.Status == "initializing" {
		return nil, nil, nil, fmt.Errorf("cluster is still initializing, wait a bit more")
	}

	if response.Credentials.Status == "error" || response.Credentials.Password == "" {
		return nil, nil, nil, fmt.Errorf("error getting cluster password")
	}

	if cluster.IpAssignments.Direct == "" {
		return nil, nil, nil, fmt.Errorf("error getting cluster IP")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, nil, nil, err
	}

	// Use the resolved organization slug for wireguard tunnel
	dialer, err := agentclient.ConnectToTunnel(ctx, resolvedOrgSlug, "", false)
	if err != nil {
		return nil, nil, nil, err
	}

	return cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "5432"},
		OrganizationSlug: resolvedOrgSlug,
		Dialer:           dialer,
		BindAddr:         flag.GetBindAddr(ctx),
		RemoteHost:       cluster.IpAssignments.Direct,
	}, &response.Credentials, nil
}
