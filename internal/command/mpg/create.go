package mpg

import (
	"context"
	"fmt"
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
	Name          string
	OrgSlug       string
	Region        string
	Plan          string
	Nodes         int
	VolumeSizeGB  int
	EnableBackups bool
	AutoStop      bool
}

func newCreate() *cobra.Command {
	const (
		short = "Create a new Managed Postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("create", short, long, runCreate,
		command.RequireSession,
		command.RequireUiex,
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
			Name:        "nodes",
			Description: "Number of nodes in the cluster",
			Default:     1,
		},
		flag.Int{
			Name:        "volume-size",
			Description: "The volume size in GB",
			Default:     10,
		},
		flag.Bool{
			Name:        "enable-backups",
			Description: "Enable WAL-based backups",
			Default:     true,
		},
		flag.Bool{
			Name:        "auto-stop",
			Description: "Automatically stop the cluster when not in use",
			Default:     false,
		},
	)

	return cmd
}

func runCreate(ctx context.Context) error {
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

	plan := flag.GetString(ctx, "plan")
	if plan == "" {
		plan = "basic" // Default plan
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
		Name:          appName,
		OrgSlug:       slug,
		Region:        selectedRegion.Code,
		Plan:          plan,
		Nodes:         flag.GetInt(ctx, "nodes"),
		VolumeSizeGB:  flag.GetInt(ctx, "volume-size"),
		EnableBackups: flag.GetBool(ctx, "enable-backups"),
		AutoStop:      flag.GetBool(ctx, "auto-stop"),
	}

	uiexClient := uiexutil.ClientFromContext(ctx)

	input := uiex.CreateClusterInput{
		Name:    params.Name,
		Region:  params.Region,
		Plan:    params.Plan,
		OrgSlug: params.OrgSlug,
	}

	response, err := uiexClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("failed creating managed postgres cluster: %w", err)
	}

	// Wait for cluster to be ready
	fmt.Fprintf(io.Out, "Waiting for cluster %s (%s) to be ready...\n", params.Name, response.Data.Id)
	fmt.Fprintf(io.Out, "You can view the cluster in the UI at: https://fly.io/dashboard/%s/managed_postgres/%s\n", params.OrgSlug, response.Data.Id)
	fmt.Fprintf(io.Out, "You can cancel this wait with Ctrl+C - the cluster will continue provisioning in the background.\n")
	fmt.Fprintf(io.Out, "Once ready, you can connect to the database with: fly mpg connect --cluster %s\n\n", response.Data.Id)
	for {
		cluster, err := uiexClient.GetManagedClusterById(ctx, response.Data.Id)
		if err != nil {
			return fmt.Errorf("failed checking cluster status: %w", err)
		}

		if cluster.Data.Id == "" {
			return fmt.Errorf("invalid cluster response: no cluster ID")
		}

		if cluster.Data.Status == "ready" {
			break
		}

		if cluster.Data.Status == "error" {
			return fmt.Errorf("cluster creation failed")
		}

		time.Sleep(5 * time.Second)
	}

	// Create a default user to get the connection string
	userInput := uiex.CreateUserInput{
		DbName:   "postgres",
		UserName: "postgres",
	}

	userResponse, err := uiexClient.CreateUser(ctx, response.Data.Id, userInput)
	if err != nil {
		return fmt.Errorf("failed creating default user: %w", err)
	}

	fmt.Fprintf(io.Out, "\nManaged Postgres cluster created successfully!\n")
	fmt.Fprintf(io.Out, "  ID: %s\n", response.Data.Id)
	fmt.Fprintf(io.Out, "  Name: %s\n", params.Name)
	fmt.Fprintf(io.Out, "  Organization: %s\n", params.OrgSlug)
	fmt.Fprintf(io.Out, "  Region: %s\n", params.Region)
	fmt.Fprintf(io.Out, "  Plan: %s\n", params.Plan)
	fmt.Fprintf(io.Out, "  Disk: %dGB\n", response.Data.Disk)
	fmt.Fprintf(io.Out, "  Connection string: %s\n", userResponse.ConnectionUri)

	return nil
}
