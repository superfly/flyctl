package cmdv1

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/proxy"
)

func RunProxy(ctx context.Context, clusterID string, resolvedOrgSlug string) error {
	localProxyPort := flag.GetString(ctx, flagnames.LocalPort)
	_, params, _, err := GetMpgProxyParams(ctx, localProxyPort, "", clusterID, resolvedOrgSlug)
	if err != nil {
		return err
	}

	return proxy.Connect(ctx, params)
}

// GetMpgProxyParams builds proxy connection parameters for a given cluster.
// resolvedOrgSlug should already be the aliased slug suitable for wireguard tunnels.
func GetMpgProxyParams(ctx context.Context, localProxyPort string, username string, clusterID string, resolvedOrgSlug string) (*uiex.ManagedCluster, *proxy.ConnectParams, *uiex.GetManagedClusterCredentialsResponse, error) {
	client := flyutil.ClientFromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	// Get cluster details
	response, err := uiexClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	cluster := &response.Data

	// Get credentials - use user-specific endpoint if username provided, otherwise use default
	var credentials uiex.GetManagedClusterCredentialsResponse
	if username != "" {
		userCreds, err := uiexClient.GetUserCredentials(ctx, cluster.Id, username)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}
		// Convert user credentials to the standard format
		credentials = uiex.GetManagedClusterCredentialsResponse{
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
