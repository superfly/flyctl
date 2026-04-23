package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command/mpg/utils"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/proxy"
)

func RunProxy(ctx context.Context, clusterID string, proxyPort string, orgSlug string) (err error) {
	_, params, _, err := getMpgProxyParams(ctx, clusterID, proxyPort, "", orgSlug)
	if err != nil {
		return err
	}

	return proxy.Connect(ctx, params)
}

func getMpgProxyParams(
	ctx context.Context,
	clusterID string,
	localProxyPort string,
	username string,
	orgSlug string,
) (*mpgv1.ManagedCluster, *proxy.ConnectParams, *mpgv1.GetManagedClusterCredentialsResponse, error) {
	client := flyutil.ClientFromContext(ctx)
	mpgClient := mpgv1.ClientFromContext(ctx)

	// Get cluster details
	response, err := mpgClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	cluster := &response.Data

	// Get credentials - use user-specific endpoint if username provided, otherwise use default
	var credentials mpgv1.GetManagedClusterCredentialsResponse
	if username != "" {
		userCreds, err := mpgClient.GetUserCredentials(ctx, cluster.Id, username)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}
		// Convert user credentials to the standard format
		credentials = mpgv1.GetManagedClusterCredentialsResponse{
			User:     userCreds.Data.User,
			Password: userCreds.Data.Password,
			DBName:   response.Credentials.DBName, // Use default DB name from cluster credentials
		}
	} else {
		credentials = response.Credentials
	}

	// Validate cluster state (only for default credentials, user credentials don't have status)
	if username == "" {
		if credentials.Status == "initializing" {
			return nil, nil, nil, fmt.Errorf("cluster is still initializing, wait a bit more")
		}

		if credentials.Status == "error" || credentials.Password == "" {
			return nil, nil, nil, fmt.Errorf("error getting cluster password")
		}
	} else if credentials.Password == "" {
		return nil, nil, nil, fmt.Errorf("error getting user password")
	}

	if cluster.IpAssignments.Direct == "" {
		return nil, nil, nil, fmt.Errorf("error getting cluster IP")
	}

	// Resolve organization slug to handle aliases
	resolvedOrgSlug, err := utils.AliasedOrganizationSlug(ctx, orgSlug)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve organization slug: %w", err)
	}

	// Establish wireguard tunnel
	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, nil, nil, err
	}

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
	}, &credentials, nil
}
