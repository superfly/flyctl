package cmdv2

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mpgutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

type CreateClusterParams struct {
	Name           string
	OrgSlug        string
	Plan           string
	StorageInGb    int
	PGMajorVersion int
	PostGISEnabled bool
}

type CreatePlanDisplay struct {
	Name       string
	CPU        string
	Memory     string
	PricePerMo int
}

// flyUser and flyDB are the fixed credentials used by the public
// Machines API's create/credentials response. The public credentials
// endpoint carries no database name, so the connection URI is composed
// against the literal "fly-db" database.
const (
	flyUser = "fly-user"
	flyDB   = "fly-db"
)

// RunCreate provisions a new Managed Postgres cluster via the public
// Machines API (flaps) and prints the connection string once ready.
//
// Unlike the other migrated MPG commands, this one deliberately does NOT
// fall back to the private client on a failed or ambiguous create call:
// retrying a mutating create against a different backend risks
// double-provisioning a cluster. Errors from CreateManagedPostgresCluster
// are propagated to the caller as-is.
//
// orgRawSlug is accepted for API parity with the previous implementation;
// region discovery is now driven by the non-org-scoped Machines API region
// list (mpgutil.AvailableRegions).
func RunCreate(ctx context.Context, orgRawSlug string, params *CreateClusterParams, planDisplay *CreatePlanDisplay) error {
	_ = orgRawSlug

	io := iostreams.FromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	// Get available MPG regions from the public Machines API region list.
	mpgRegions, err := mpgutil.AvailableRegions(ctx)
	if err != nil {
		return err
	}

	if len(mpgRegions) == 0 {
		return fmt.Errorf("no valid regions found for Managed Postgres")
	}

	// Check if region was specified via flag
	regionCode := flag.GetString(ctx, "region")
	var selectedRegionCode string

	if regionCode != "" {
		// Find the specified region in the allowed regions
		var matched bool
		for _, region := range mpgRegions {
			if region.Code == regionCode {
				selectedRegionCode = region.Code
				matched = true

				break
			}
		}
		if !matched {
			availableCodes := make([]string, len(mpgRegions))
			for i, region := range mpgRegions {
				availableCodes[i] = region.Code
			}

			return fmt.Errorf("region %s is not available for Managed Postgres. Available regions: %v", regionCode, availableCodes)
		}
	} else {
		// Create region options for prompt
		var regionOptions []string
		for _, region := range mpgRegions {
			regionOptions = append(regionOptions, fmt.Sprintf("%s (%s)", region.Name, region.Code))
		}

		var selectedIndex int
		if err := prompt.Select(ctx, &selectedIndex, "Select a region for your Managed Postgres cluster", "", regionOptions...); err != nil {
			return err
		}

		selectedRegionCode = mpgRegions[selectedIndex].Code
	}

	createReq := flaps.CreateManagedPostgresClusterRequest{
		Name:           params.Name,
		Region:         selectedRegionCode,
		Plan:           params.Plan,
		OrgSlug:        params.OrgSlug,
		DiskSizeGB:     params.StorageInGb,
		PGMajorVersion: strconv.Itoa(params.PGMajorVersion),
		PostGISEnabled: params.PostGISEnabled,
	}

	cluster, err := flapsClient.CreateManagedPostgresCluster(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed creating managed postgres cluster: %w", err)
	}

	clusterID := cluster.ID

	var connectionURI string

	// Output plan details after creation
	fmt.Fprintf(io.Out, "Selected Plan: %s\n", planDisplay.Name)
	fmt.Fprintf(io.Out, "  CPU: %s\n", planDisplay.CPU)
	fmt.Fprintf(io.Out, "  Memory: %s\n", planDisplay.Memory)
	fmt.Fprintf(io.Out, "  Price: $%d per month\n\n", planDisplay.PricePerMo)

	// Wait for cluster to be ready
	fmt.Fprintf(io.Out, "Waiting for cluster %s (%s) to be ready...\n", params.Name, clusterID)
	fmt.Fprintf(io.Out, "You can view the cluster in the UI at: https://fly.io/dashboard/%s/managed_postgres/%s\n", params.OrgSlug, clusterID)
	fmt.Fprintf(io.Out, "You can cancel this wait with Ctrl+C - the cluster will continue provisioning in the background.\n")
	fmt.Fprintf(io.Out, "Once ready, you can connect to the database with: fly mpg connect %s\n\n", clusterID)
	for {
		res, err := flapsClient.GetManagedPostgresCluster(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed checking cluster status: %w", err)
		}

		if res.ID == "" {
			return fmt.Errorf("invalid cluster response: no cluster ID")
		}

		switch res.Status {
		case flaps.ManagedPostgresStatusReady:
			creds, credErr := flapsClient.GetManagedPostgresUserCredentials(ctx, clusterID, flyUser)
			if credErr != nil {
				return fmt.Errorf("failed retrieving credentials for cluster %s: %w", clusterID, credErr)
			}

			// The connection URI's username is the fixed "fly-user" the plan
			// requested credentials for, NOT whatever Username the credentials
			// response carries — invariant #6 keeps the URI user stable
			// regardless of any unexpected payload drift.
			uri, buildErr := buildConnectionURI(res.Endpoints.Primary.Pooler, flyUser, creds.Password, flyDB)
			if buildErr != nil {
				return fmt.Errorf("failed to build connection URI for cluster %s: %w", clusterID, buildErr)
			}
			connectionURI = uri

		case flaps.ManagedPostgresStatusFailed, flaps.ManagedPostgresStatusError:
			return fmt.Errorf("cluster creation failed")
		}

		if connectionURI != "" {
			break
		}

		time.Sleep(5 * time.Second)
	}

	fmt.Fprintf(io.Out, "\nManaged Postgres cluster created successfully!\n")
	fmt.Fprintf(io.Out, "  ID: %s\n", clusterID)
	fmt.Fprintf(io.Out, "  Name: %s\n", params.Name)
	fmt.Fprintf(io.Out, "  Organization: %s\n", params.OrgSlug)
	fmt.Fprintf(io.Out, "  Region: %s\n", cluster.Region)
	fmt.Fprintf(io.Out, "  Plan: %s\n", params.Plan)
	fmt.Fprintf(io.Out, "  Disk: %dGB\n", cluster.DiskSizeGB)
	fmt.Fprintf(io.Out, "  PostGIS: %t\n", cluster.PostGISEnabled)
	fmt.Fprintf(io.Out, "  Connection string: %s\n", connectionURI)

	return nil
}

// buildConnectionURI composes a pooled (PgBouncer-style) connection URI for
// the given endpoint + credentials. The password is URL-escaped via
// url.UserPassword so that reserved characters in the password do not corrupt
// the URI.
//
// The endpoint Host/Port are validated before composition: a cluster that
// reports status "ready" can still carry a zero-valued pooler endpoint during
// the propagation-lag race between readiness and endpoint population. Without
// this check, the function would silently return a broken URI such as
// "postgres://fly-user:secret@:0/fly-db" with a nil error.
func buildConnectionURI(endpoint flaps.ManagedPostgresEndpoint, username, password, dbName string) (string, error) {
	if endpoint.Host == "" || endpoint.Port == 0 {
		return "", fmt.Errorf("cluster ready but pooler endpoint not yet available")
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(username, password),
		Host:   fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port),
		Path:   "/" + dbName,
	}

	return u.String(), nil
}
