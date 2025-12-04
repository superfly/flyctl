package mpg

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
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

func newCreate() *cobra.Command {
	const (
		short = "Create a new Managed Postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("create", short, long, runCreate,
		command.RequireSession,
	)

	flag.Add(
		cmd,
		flag.Region(),
		flag.Org(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Postgres cluster",
		},
		flag.String{
			Name:        "plan",
			Description: "The plan to use for the Postgres cluster (development, production, etc)",
		},
		flag.Int{
			Name:        "volume-size",
			Description: "The volume size in GB",
			Default:     10,
		},
		flag.Bool{
			Name:        "enable-postgis-support",
			Description: "Enable PostGIS for the Postgres cluster",
			Default:     false,
		},
		flag.Int{
			Name:        "pg-major-version",
			Description: "The major version of Postgres to use for the Postgres cluster. Supported versions are 16 and 17.",
			Default:     16,
		},
	)

	return cmd
}

func runCreate(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	var (
		io      = iostreams.FromContext(ctx)
		appName = flag.GetString(ctx, "name")
		err     error
	)

	if appName == "" {
		// If no name is provided, try to get the app name from context
		if appName = appconfig.NameFromContext(ctx); appName != "" {
			// If we have an app name, use it to create a default database name
			appName = appName + "-db"
		} else {
			// If no app name is available, prompt for a name
			appName, err = prompt.SelectAppNameWithMsg(ctx, "Choose a database name:")
			if err != nil {
				return err
			}
		}
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	// Get available MPG regions from API
	mpgRegions, err := GetAvailableMPGRegions(ctx, org.RawSlug)

	if err != nil {
		return err
	}

	if len(mpgRegions) == 0 {
		return fmt.Errorf("no valid regions found for Managed Postgres")
	}

	pgMajorVersion := flag.GetInt(ctx, "pg-major-version")
	if pgMajorVersion != 16 && pgMajorVersion != 17 {
		return fmt.Errorf("invalid Postgres major version: %d. Supported versions are 16 and 17", pgMajorVersion)
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
			availableCodes, _ := GetAvailableMPGRegionCodes(ctx, org.Slug)
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

	// Plan selection and validation
	plan := flag.GetString(ctx, "plan")
	plan = normalizePlan(plan)
	if _, ok := MPGPlans[plan]; !ok {
		if iostreams.FromContext(ctx).IsInteractive() {
			// Prepare a sortable slice of plans
			type planEntry struct {
				Key   string
				Value PlanDetails
			}
			var planEntries []planEntry
			for k, v := range MPGPlans {
				planEntries = append(planEntries, planEntry{Key: k, Value: v})
			}
			// Sort by price (convert string like "$38.00" to float)
			sort.Slice(planEntries, func(i, j int) bool {
				return planEntries[i].Value.PricePerMo < planEntries[j].Value.PricePerMo
			})
			// Build options and keys in sorted order
			var planOptions []string
			var planKeys []string
			for _, entry := range planEntries {
				planOptions = append(planOptions, fmt.Sprintf("%s: %s, %s RAM, $%d/mo", entry.Value.Name, entry.Value.CPU, entry.Value.Memory, entry.Value.PricePerMo))
				planKeys = append(planKeys, entry.Key)
			}
			var selectedIndex int
			if err := prompt.Select(ctx, &selectedIndex, "Select a plan for your Managed Postgres cluster", planOptions[0], planOptions...); err != nil {
				return err
			}
			plan = planKeys[selectedIndex]
		} else {
			plan = "basic" // Default to basic if not interactive
		}
	}

	var slug string
	if org.Slug == "personal" {
		genqClient := flyutil.ClientFromContext(ctx).GenqClient()

		// For ui-ex request we need the real org slug
		var fullOrg *gql.GetOrganizationResponse
		if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
			return fmt.Errorf("failed fetching org: %w", err)
		}

		slug = fullOrg.Organization.RawSlug
	} else {
		slug = org.Slug
	}

	params := &CreateClusterParams{
		Name:           appName,
		OrgSlug:        slug,
		Region:         selectedRegion.Code,
		Plan:           plan,
		VolumeSizeGB:   flag.GetInt(ctx, "volume-size"),
		PostGISEnabled: flag.GetBool(ctx, "enable-postgis-support"),
		PGMajorVersion: pgMajorVersion,
	}

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
	planDetails := MPGPlans[plan]
	fmt.Fprintf(io.Out, "Selected Plan: %s\n", planDetails.Name)
	fmt.Fprintf(io.Out, "  CPU: %s\n", planDetails.CPU)
	fmt.Fprintf(io.Out, "  Memory: %s\n", planDetails.Memory)
	fmt.Fprintf(io.Out, "  Price: $%d per month\n\n", planDetails.PricePerMo)

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

// normalizePlan lowercases and trims whitespace from the plan name for lookup
func normalizePlan(plan string) string {
	return strings.ToLower(strings.TrimSpace(plan))
}
