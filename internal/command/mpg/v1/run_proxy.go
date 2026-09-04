package cmdv1

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mpgutil"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
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
) (*mpgv1.ManagedCluster, *proxy.ConnectParams, error) {
	response, _, port, err := getCluster(ctx, clusterID)
	if err != nil {
		return nil, nil, err
	}

	cluster, params, err := buildProxyParams(ctx, response, port, localProxyPort, resolvedOrgSlug)
	if err != nil {
		return nil, nil, err
	}

	return cluster, params, nil
}

// GetMpgConnectParams resolves credentials and proxy parameters.
func GetMpgConnectParams(
	ctx context.Context,
	localProxyPort string,
	username string,
	clusterID string,
	resolvedOrgSlug string,
) (*mpgv1.ManagedCluster, *proxy.ConnectParams, *mpgv1.GetManagedClusterCredentialsResponse, error) {
	response, useLegacy, port, err := getCluster(ctx, clusterID)
	if err != nil {
		return nil, nil, nil, err
	}

	credentials, err := resolveConnectCredentials(ctx, response, useLegacy, username)
	if err != nil {
		return nil, nil, nil, err
	}

	cluster, params, err := buildProxyParams(ctx, response, port, localProxyPort, resolvedOrgSlug)
	if err != nil {
		return nil, nil, nil, err
	}

	return cluster, params, credentials, nil
}

// getCluster tries the public API, falling back to the legacy client only on 404.
// It returns the credential source and direct endpoint port (5432 for legacy).
func getCluster(ctx context.Context, clusterID string) (*mpgv1.GetManagedClusterResponse, bool, int, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	publicCluster, err := flapsClient.GetManagedPostgresCluster(ctx, clusterID)
	if err == nil {
		response, port := publicToLegacyClusterResponse(publicCluster)

		return &response, false, port, nil
	}

	if !errors.Is(err, flaps.ErrFlapsNotFound) {
		return nil, false, 0, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	legacyClient := mpgv1.ClientFromContext(ctx)
	response, err := legacyClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return nil, true, 0, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	return &response, true, mpgutil.DefaultPort, nil
}

// publicToLegacyClusterResponse adapts the public cluster to the legacy shape.
// The advertised direct port is returned unchanged for validation.
func publicToLegacyClusterResponse(c flaps.ManagedPostgresCluster) (mpgv1.GetManagedClusterResponse, int) {
	port := c.Endpoints.Primary.Direct.Port

	return mpgv1.GetManagedClusterResponse{
		Data: mpgv1.ManagedCluster{
			Id:            c.ID,
			Name:          c.Name,
			Status:        c.Status,
			Region:        c.Region,
			Plan:          c.Plan,
			Disk:          c.DiskSizeGB,
			Replicas:      c.Replicas,
			Organization:  fly.Organization{Name: c.Organization.Name, Slug: c.Organization.Slug},
			IpAssignments: mpg.ManagedClusterIpAssignments{Direct: c.Endpoints.Primary.Direct.Host},
		},
	}, port
}

// resolveConnectCredentials uses the same API as the cluster lookup.
// Public credentials default to fly-user and fly-db.
func resolveConnectCredentials(
	ctx context.Context,
	response *mpgv1.GetManagedClusterResponse,
	useLegacy bool,
	username string,
) (*mpgv1.GetManagedClusterCredentialsResponse, error) {
	var credentials mpgv1.GetManagedClusterCredentialsResponse

	switch {
	case username != "" && useLegacy:
		mpgClient := mpgv1.ClientFromContext(ctx)
		userCreds, err := mpgClient.GetUserCredentials(ctx, response.Data.Id, username)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}

		credentials = mpgv1.GetManagedClusterCredentialsResponse{
			User:     userCreds.Data.User,
			Password: userCreds.Data.Password,
			DBName:   response.Credentials.DBName,
		}
	case username != "":
		flapsClient := flapsutil.ClientFromContext(ctx)
		userCreds, err := flapsClient.GetManagedPostgresUserCredentials(ctx, response.Data.Id, username)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving credentials for user %s: %w", username, err)
		}

		credentials = mpgv1.GetManagedClusterCredentialsResponse{
			User:     userCreds.Username,
			Password: userCreds.Password,
			DBName:   mpgutil.DefaultDatabase,
		}
	case useLegacy:
		credentials = response.Credentials
	default:
		flapsClient := flapsutil.ClientFromContext(ctx)
		userCreds, err := flapsClient.GetManagedPostgresUserCredentials(ctx, response.Data.Id, mpgutil.DefaultUsername)
		if err != nil {
			if errors.Is(err, flaps.ErrFlapsNotFound) {
				return nil, fmt.Errorf("cluster is still initializing, wait a bit more")
			}

			return nil, fmt.Errorf("failed retrieving credentials for user %s: %w", mpgutil.DefaultUsername, err)
		}

		credentials = mpgv1.GetManagedClusterCredentialsResponse{
			User:     userCreds.Username,
			Password: userCreds.Password,
			DBName:   mpgutil.DefaultDatabase,
		}
	}

	if useLegacy {
		// Only legacy default-user credentials include a status.
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
	} else if credentials.Password == "" {
		if username == "" {
			return nil, fmt.Errorf("error getting cluster password")
		}

		return nil, fmt.Errorf("error getting user password")
	}

	return &credentials, nil
}

func buildProxyParams(
	ctx context.Context,
	response *mpgv1.GetManagedClusterResponse,
	port int,
	localProxyPort string,
	resolvedOrgSlug string,
) (*mpgv1.ManagedCluster, *proxy.ConnectParams, error) {
	cluster, params, err := proxyParams(response, port, localProxyPort, resolvedOrgSlug, flag.GetBindAddr(ctx), nil)
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
	response *mpgv1.GetManagedClusterResponse,
	port int,
	localProxyPort string,
	resolvedOrgSlug string,
	bindAddr string,
	dialer agent.Dialer,
) (*mpgv1.ManagedCluster, *proxy.ConnectParams, error) {
	cluster := &response.Data
	if cluster.IpAssignments.Direct == "" {
		return nil, nil, fmt.Errorf("error getting cluster IP")
	}

	if port < 1 || port > 65535 {
		return nil, nil, fmt.Errorf("invalid cluster port %d: must be between 1 and 65535", port)
	}

	return cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, strconv.Itoa(port)},
		OrganizationSlug: resolvedOrgSlug,
		Dialer:           dialer,
		BindAddr:         bindAddr,
		RemoteHost:       cluster.IpAssignments.Direct,
	}, nil
}
