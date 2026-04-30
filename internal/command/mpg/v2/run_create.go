package cmdv2

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/superfly/fly-go"
	regionsv2 "github.com/superfly/flyctl/internal/command/mpg/v2/regions"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

type CreateClusterParams struct {
	Name           string
	OrgSlug        string
	Plan           string
	Region         string
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

func RunCreate(ctx context.Context, orgRawSlug string, params *CreateClusterParams, planDisplay *CreatePlanDisplay) error {
	io := iostreams.FromContext(ctx)
	mpgClient := mpgv2.ClientFromContext(ctx)

	// Get available MPG regions from API
	mpgRegions, err := regionsv2.GetAvailableMPGRegions(ctx, orgRawSlug)
	if err != nil {
		return err
	}

	if len(mpgRegions) == 0 {
		return fmt.Errorf("no valid regions found for Managed Postgres")
	}

	// Check if region was specified via flag
	regionCode := flag.GetString(ctx, "region")
	var selectedRegion *fly.Region

	if regionCode != "" {
		// Find the specified region in the allowed regions
		for _, region := range mpgRegions {
			if region.Code == regionCode {
				selectedRegion = &region

				break
			}
		}
		if selectedRegion == nil {
			availableCodes, _ := regionsv2.GetAvailableMPGRegionCodes(ctx, params.OrgSlug)

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

		selectedRegion = &mpgRegions[selectedIndex]
	}

	input := mpgv2.CreateClusterInput{
		Name:           params.Name,
		Region:         selectedRegion.Code,
		Plan:           params.Plan,
		OrgSlug:        params.OrgSlug,
		StorageInGb:    params.StorageInGb,
		PostGISEnabled: params.PostGISEnabled,
		PGMajorVersion: strconv.Itoa(params.PGMajorVersion),
	}

	response, err := mpgClient.CreateCluster(ctx, input)
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
		res, err := mpgClient.GetClusterById(ctx, clusterID)
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
