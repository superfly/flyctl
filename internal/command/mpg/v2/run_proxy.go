package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/proxy"
)

func RunProxy(ctx context.Context, clusterID string, resolvedOrgSlug string, proxyPort string) error {
	_, params, err := GetMpgProxyParams(ctx, proxyPort, clusterID, resolvedOrgSlug)
	if err != nil {
		return err
	}

	return proxy.Connect(ctx, params)
}

// GetMpgProxyParams builds proxy connection parameters for a given cluster
// without requiring database credentials.
// resolvedOrgSlug should already be the aliased slug suitable for wireguard tunnels.
func GetMpgProxyParams(
	ctx context.Context,
	localProxyPort string,
	clusterID string,
	resolvedOrgSlug string,
) (*mpgv2.ManagedCluster, *proxy.ConnectParams, error) {
	response, err := getCluster(ctx, clusterID)
	if err != nil {
		return nil, nil, err
	}

	cluster, params, err := buildProxyParams(ctx, response, localProxyPort, resolvedOrgSlug)
	if err != nil {
		return nil, nil, err
	}

	return cluster, params, nil
}

// GetMpgConnectParams builds proxy connection parameters and resolves the
// database credentials needed by fly mpg connect.
func GetMpgConnectParams(
	ctx context.Context,
	localProxyPort string,
	username string,
	clusterID string,
	resolvedOrgSlug string,
) (*mpgv2.ManagedCluster, *proxy.ConnectParams, *mpgv2.GetClusterCredentialsResponse, error) {
	response, err := getCluster(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, err
	}

	credentials, err := resolveConnectCredentials(ctx, response, username)
	if err != nil {
		return nil, nil, nil, err
	}

	cluster, params, err := buildProxyParams(ctx, response, localProxyPort, resolvedOrgSlug)
	if err != nil {
		return nil, nil, nil, err
	}

	return cluster, params, credentials, nil
}

func getCluster(ctx context.Context, clusterID string) (*mpgv2.GetClusterResponse, error) {
	mpgClient := mpgv2.ClientFromContext(ctx)
	response, err := mpgClient.GetClusterById(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	return &response, nil
}

func resolveConnectCredentials(
	ctx context.Context,
	response *mpgv2.GetClusterResponse,
	username string,
) (*mpgv2.GetClusterCredentialsResponse, error) {
	var credentials mpgv2.GetClusterCredentialsResponse
	if username != "" {
		mpgClient := mpgv2.ClientFromContext(ctx)
		userCreds, err := mpgClient.GetUserCredentials(ctx, response.Data.Id, username)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}

		credentials = mpgv2.GetClusterCredentialsResponse{
			User:     userCreds.Data.User,
			Password: userCreds.Data.Password,
			DBName:   response.Credentials.DBName,
		}
	} else {
		credentials = response.Credentials
	}

	if username == "" {
		if credentials.Status == "initializing" {
			return nil, fmt.Errorf("cluster is still initializing, wait a bit more")
		}

		if credentials.Status == "error" || credentials.Password == "" {
			return nil, fmt.Errorf("error getting cluster password")
		}
	} else if credentials.Password == "" {
		return nil, fmt.Errorf("error getting user password")
	}

	return &credentials, nil
}

func buildProxyParams(
	ctx context.Context,
	response *mpgv2.GetClusterResponse,
	localProxyPort string,
	resolvedOrgSlug string,
) (*mpgv2.ManagedCluster, *proxy.ConnectParams, error) {
	cluster, params, err := proxyParams(response, localProxyPort, resolvedOrgSlug, flag.GetBindAddr(ctx), nil)
	if err != nil {
		return nil, nil, err
	}

	client := flyutil.ClientFromContext(ctx)

	// Establish wireguard tunnel after validating all prerequisites.
	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, nil, err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, resolvedOrgSlug, "", false)
	if err != nil {
		return nil, nil, err
	}

	params.Dialer = dialer

	return cluster, params, nil
}

func proxyParams(
	response *mpgv2.GetClusterResponse,
	localProxyPort string,
	resolvedOrgSlug string,
	bindAddr string,
	dialer agent.Dialer,
) (*mpgv2.ManagedCluster, *proxy.ConnectParams, error) {
	cluster := &response.Data
	if cluster.IpAssignments.Direct == "" {
		return nil, nil, fmt.Errorf("error getting cluster IP")
	}

	return cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "5432"},
		OrganizationSlug: resolvedOrgSlug,
		Dialer:           dialer,
		BindAddr:         bindAddr,
		RemoteHost:       cluster.IpAssignments.Direct,
	}, nil
}
