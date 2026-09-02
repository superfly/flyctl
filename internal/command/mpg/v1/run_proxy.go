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
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/proxy"
)

// defaultMPGUser is the literal username used when resolving the default
// fly-user on the public Machines API for Connect. Verified against the
// orchestrator and ui-ex repos.
const defaultMPGUser = "fly-user"

// defaultMPGDatabase is the literal database name used when no --database
// flag is given and no interactive prompt answer is available.
const defaultMPGDatabase = "fly-db"

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

// GetMpgConnectParams builds proxy connection parameters and resolves the
// database credentials needed by fly mpg connect.
//
// The returned useLegacy bool mirrors the value computed inside getCluster
// (true when the public Machines API returned a classified 404 and we fell
// back to the legacy ui-ex client; false when the public API succeeded).
// RunConnect uses it to gate the pre-migration "Cluster is not in ready
// state" stderr warning, which is meaningful only on the legacy path —
// see connectStatusRefusal's doc comment for why the public path does not
// need it.
func GetMpgConnectParams(
	ctx context.Context,
	localProxyPort string,
	username string,
	clusterID string,
	resolvedOrgSlug string,
) (*mpgv1.ManagedCluster, bool, *proxy.ConnectParams, *mpgv1.GetManagedClusterCredentialsResponse, error) {
	response, useLegacy, port, err := getCluster(ctx, clusterID)
	if err != nil {
		return nil, false, nil, nil, err
	}

	credentials, err := resolveConnectCredentials(ctx, response, useLegacy, username)
	if err != nil {
		return nil, false, nil, nil, err
	}

	cluster, params, err := buildProxyParams(ctx, response, port, localProxyPort, resolvedOrgSlug)
	if err != nil {
		return nil, false, nil, nil, err
	}

	return cluster, useLegacy, params, credentials, nil
}

// getCluster retrieves cluster details via the public Machines API, falling
// back to the legacy MPGv1 client only when the public call returns a
// classified 404. Any other non-nil public error is returned immediately,
// wrapped in "failed retrieving cluster %s: %w". The returned useLegacy flag
// indicates which client
// produced the response so downstream credential resolution can choose the
// appropriate code path. The returned port is the public API's advertised
// direct-endpoint port (nil on the legacy path, which has no port field) —
// carried as a separate return value rather than a field on the legacy
// mpgv1.ManagedCluster type, since that type is part of the private/legacy
// client package (internal/uiex/mpg) that this migration must not modify.
func getCluster(ctx context.Context, clusterID string) (*mpgv1.GetManagedClusterResponse, bool, *int, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	publicCluster, err := flapsClient.GetManagedPostgresCluster(ctx, clusterID)
	if err == nil {
		response, port := publicToLegacyClusterResponse(publicCluster)
		return &response, false, port, nil
	}

	if !errors.Is(err, flaps.ErrFlapsNotFound) {
		return nil, false, nil, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	legacyClient := mpgv1.ClientFromContext(ctx)
	response, err := legacyClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return nil, true, nil, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}

	return &response, true, nil, nil
}

// publicToLegacyClusterResponse wraps the public Machines API cluster in the
// legacy ui-ex envelope so that downstream callers (proxyParams,
// resolveConnectCredentials) can read IpAssignments.Direct and Status without
// translation. The legacy shape preserves the bare-address column for
// proxyParams.RemoteHost and the Status field for connectStatusRefusal.
//
// The public API's advertised Endpoints.Primary.Direct.Port is returned
// separately (not as a field on the legacy mpgv1.ManagedCluster type) so
// proxyParams can dial the real advertised port instead of the legacy
// hardcoded 5432, without adding a public-API-only field to the private
// legacy client package. A nil port return means "no port info" (the legacy
// path never calls this function at all, so in practice this only happens
// if ever called with a zero-value input); a non-nil pointer — even to 0 —
// means the public path returned some value, and *0 is treated as invalid
// by proxyParams.
func publicToLegacyClusterResponse(c flaps.ManagedPostgresCluster) (mpgv1.GetManagedClusterResponse, *int) {
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
	}, &port
}

// resolveConnectCredentials returns the credentials used by RunConnect.
// useLegacy=true routes through the legacy ui-ex client; useLegacy=false
// routes through the public Machines API's
// GetManagedPostgresUserCredentials. The default user (no --user flag)
// resolves to defaultMPGUser ("fly-user"); explicit users pass the flag
// value straight through. The public path has no envelope-level DBName,
// so both public branches fall back to defaultMPGDatabase ("fly-db") —
// which run_connect.go's buildConnectURL uses to land on the plan-required
// default when neither --database nor an interactive prompt supplies one.
//
// Status gating is split across the two paths because their vocabularies
// differ; see connectStatusRefusal's doc comment for the full rationale.
// Briefly: the public path consults response.Data.Status BEFORE any
// credentials call (a 7-value enum + fail-closed default); the legacy
// path keeps the pre-migration post-fetch credentials.Status +
// credentials.Password check ("error" is a real legacy value but not a
// public one, and the legacy envelope's Status field is semantically
// distinct from cluster status).
func resolveConnectCredentials(
	ctx context.Context,
	response *mpgv1.GetManagedClusterResponse,
	useLegacy bool,
	username string,
) (*mpgv1.GetManagedClusterCredentialsResponse, error) {
	// Status classifier runs BEFORE any credentials call on the public
	// path only — see connectStatusRefusal's doc comment for the
	// legacy/public split. response.Data.Status is populated on the public
	// path via publicToLegacyClusterResponse (from c.Status); it is
	// intentionally NOT consulted on the legacy path.
	if !useLegacy {
		if err := connectStatusRefusal(response.Data.Name, response.Data.Status); err != nil {
			return nil, err
		}
	}

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
			DBName:   defaultMPGDatabase,
		}
	case useLegacy:
		credentials = response.Credentials
	default:
		flapsClient := flapsutil.ClientFromContext(ctx)
		userCreds, err := flapsClient.GetManagedPostgresUserCredentials(ctx, response.Data.Id, defaultMPGUser)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving credentials for user %s: %w", defaultMPGUser, err)
		}

		credentials = mpgv1.GetManagedClusterCredentialsResponse{
			User:     userCreds.Username,
			Password: userCreds.Password,
			DBName:   defaultMPGDatabase,
		}
	}

	if useLegacy {
		// ORIGINAL pre-migration legacy status/password checks,
		// restored verbatim. credentials.Status is only checked on the
		// default-user path because the explicit-user legacy
		// GetUserCredentials response does not populate that field
		// (only User/Password/DBName). See connectStatusRefusal's doc
		// comment for why this path is intentionally distinct from the
		// public classifier.
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
		// Public path: empty-password fallback after the credentials
		// call. The wording tracks whether the user was explicit
		// (flag/prompt) or defaulted to the plan-required fly-user,
		// preserving the existing pre-fix messages. The public path
		// has no envelope-level Status to inspect post-fetch, so the
		// pre-credential connectStatusRefusal call above is the only
		// status gate.
		if username == "" {
			return nil, fmt.Errorf("error getting cluster password")
		}

		return nil, fmt.Errorf("error getting user password")
	}

	return &credentials, nil
}

// connectStatusRefusal is the explicit status classifier for fly mpg
// connect. It returns nil when the cluster's status permits a connect
// attempt and a deliberate, status-specific refusal error otherwise. The
// complete emitted set of "status" values from a successful public MPG
// cluster lookup (traced across the mpg / nomad-firecracker / ui-ex code
// paths this session) is:
//
//	ready, standby_ready, creating, deleting, failed, deleted, initializing
//
// "error" is NOT a real public cluster status value; it does not appear
// in the public enum and is rejected if encountered. Behavior per status:
//
//   - ready:           proceed, no message.
//   - standby_ready:   refuse — a standby replica is not a connect target.
//     Fail-closed pending a product decision on standby
//     semantics; not even a "warn and continue" — refused
//     outright.
//   - creating:        refuse — friendly wording distinct from initializing
//     since the cluster is in the process of coming up,
//     not stuck mid-provision.
//   - deleting:        refuse — cluster is going away, connections
//     meaningless.
//   - deleted:         refuse — terminal state, no connect possible.
//   - failed:          refuse — accurate wording ("cluster is in a failed
//     state"), not the misleading "error getting cluster
//     password" misdiagnosis.
//   - initializing:    refuse — neutral wording because this now covers
//     many underlying states (degraded, updating,
//     resizing, credential rotation, promotion, and
//     future unmapped states), not just fresh
//     provisioning. Do not over-promise "wait a bit more"
//     for a state that may not be purely transient.
//   - default:         refuse — unknown / unrecognized statuses fail
//     closed with the actual status string quoted so the
//     user (and on-call) can see what the public API
//     actually returned. Do NOT let unknown values fall
//     through to credential resolution.
//
// The classifier applies ONLY to the public path (useLegacy == false),
// BEFORE any public credential call, on both the default-user and
// explicit-user paths. It is intentionally NOT applied to the legacy
// path (useLegacy == true): the legacy status vocabulary differs from
// the public API's — e.g. "error" is a real legacy cluster status value
// but not a public one — and the pre-migration legacy code understood it
// natively via credentials.Status (the legacy credentials envelope's own
// status field, populated post-fetch and semantically distinct from
// response.Data.Status). Applying this public-only classifier to legacy
// responses would mis-handle legacy-only statuses as "unrecognized
// state" and silently drop the credentials.Status check.
//
// The classifier is NOT applied to fly mpg proxy — that command
// deliberately does not gate on status because the raw TCP proxy never
// uses credentials, only the direct IP/port; see RunProxy /
// GetMpgProxyParams. Adding a status gate there would reintroduce the
// exact coupling flyctl#5048 was built to remove.
//
// name is the cluster identifier included in the refusal message so the
// user knows which cluster the refusal applies to. It comes from
// response.Data.Name (populated on both the public and legacy paths).
func connectStatusRefusal(name, status string) error {
	switch status {
	case "ready":
		return nil
	case "standby_ready":
		return fmt.Errorf("cluster %s is a standby replica and cannot be used with fly mpg connect", name)
	case "creating":
		return fmt.Errorf("cluster %s is still being created, wait a bit more", name)
	case "deleting":
		return fmt.Errorf("cluster %s is being deleted and cannot be connected to", name)
	case "deleted":
		return fmt.Errorf("cluster %s has been deleted and cannot be connected to", name)
	case "failed":
		return fmt.Errorf("cluster %s is in a failed state", name)
	case "initializing":
		return fmt.Errorf("cluster %s is not currently ready for connections (status: initializing)", name)
	default:
		return fmt.Errorf("cluster %s is in an unrecognized state (%q) and cannot be connected to", name, status)
	}
}

func buildProxyParams(
	ctx context.Context,
	response *mpgv1.GetManagedClusterResponse,
	port *int,
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
	port *int,
	localProxyPort string,
	resolvedOrgSlug string,
	bindAddr string,
	dialer agent.Dialer,
) (*mpgv1.ManagedCluster, *proxy.ConnectParams, error) {
	cluster := &response.Data
	if cluster.IpAssignments.Direct == "" {
		return nil, nil, fmt.Errorf("error getting cluster IP")
	}

	// remotePort is the port the proxy will dial on the remote host. The
	// legacy path has no port field at all, so it always calls this with
	// port == nil and we fall back to "5432" exactly as before. The public
	// path passes the advertised Endpoints.Primary.Direct.Port through
	// getCluster so a non-5432 public port dials that real port instead of
	// being silently rewritten to 5432. A public API that advertises port 0
	// is treated as an invalid/unexpected value and surfaces as an error —
	// distinguishing it from the legacy "no port field" case, where 5432 is
	// the historical default and the safe fallback. The *int rather than a
	// separate bool encodes the "optional value" semantic directly: nil =
	// no port info (legacy), non-nil (even *port == 0) = public path
	// advertised some value.
	remotePort := "5432"
	if port != nil {
		if *port == 0 {
			return nil, nil, fmt.Errorf("error getting cluster port")
		}

		remotePort = strconv.Itoa(*port)
	}

	return cluster, &proxy.ConnectParams{
		Ports:            []string{localProxyPort, remotePort},
		OrganizationSlug: resolvedOrgSlug,
		Dialer:           dialer,
		BindAddr:         bindAddr,
		RemoteHost:       cluster.IpAssignments.Direct,
	}, nil
}
