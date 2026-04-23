package cmdv1

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

type CreateClusterParams struct {
	Name           string
	OrgSlug        string
	Region         string
	Plan           string
	VolumeSizeGB   int
	PostGISEnabled bool
	PGMajorVersion int
}

type CreatePlanDisplay struct {
	Name       string
	CPU        string
	Memory     string
	PricePerMo int
}

func RunCreate(ctx context.Context, params *CreateClusterParams, planDisplay *CreatePlanDisplay) error {
	io := iostreams.FromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	input := uiex.CreateClusterInput{
		Name:           params.Name,
		Region:         params.Region,
		Plan:           params.Plan,
		OrgSlug:        params.OrgSlug,
		Disk:           params.VolumeSizeGB,
		PostGISEnabled: params.PostGISEnabled,
		PGMajorVersion: strconv.Itoa(params.PGMajorVersion),
	}

	response, err := uiexClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("failed creating managed postgres cluster: %w", err)
	}

	clusterID := response.Data.Id

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
	fmt.Fprintf(io.Out, "Once ready, you can connect to the database with: fly mpg connect --cluster %s\n\n", clusterID)
	for {
		res, err := uiexClient.GetManagedClusterById(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed checking cluster status: %w", err)
		}

		cluster := res.Data
		credentials := res.Credentials

		if cluster.Id == "" {
			return fmt.Errorf("invalid cluster response: no cluster ID")
		}

		if cluster.Status == "ready" {
			connectionURI = credentials.ConnectionUri

			break
		}

		if cluster.Status == "error" {
			return fmt.Errorf("cluster creation failed")
		}

		time.Sleep(5 * time.Second)
	}

	fmt.Fprintf(io.Out, "\nManaged Postgres cluster created successfully!\n")
	fmt.Fprintf(io.Out, "  ID: %s\n", clusterID)
	fmt.Fprintf(io.Out, "  Name: %s\n", params.Name)
	fmt.Fprintf(io.Out, "  Organization: %s\n", params.OrgSlug)
	fmt.Fprintf(io.Out, "  Region: %s\n", params.Region)
	fmt.Fprintf(io.Out, "  Plan: %s\n", params.Plan)
	fmt.Fprintf(io.Out, "  Disk: %dGB\n", response.Data.Disk)
	fmt.Fprintf(io.Out, "  PostGIS: %t\n", response.Data.PostGISEnabled)
	fmt.Fprintf(io.Out, "  Connection string: %s\n", connectionURI)

	return nil
}
