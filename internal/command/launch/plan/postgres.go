package plan

import (
	"context"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/mpg"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

type PostgresPlan struct {
	FlyPostgres      *FlyPostgresPlan      `json:"fly_postgres"`
	SupabasePostgres *SupabasePostgresPlan `json:"supabase_postgres"`
	ManagedPostgres  *ManagedPostgresPlan  `json:"managed_postgres"`
}

func (p *PostgresPlan) Provider() any {
	if p == nil {
		return nil
	}
	if p.FlyPostgres != nil {
		return p.FlyPostgres
	}
	if p.SupabasePostgres != nil {
		return p.SupabasePostgres
	}
	if p.ManagedPostgres != nil {
		return p.ManagedPostgres
	}
	return nil
}

// DefaultPostgres returns the default postgres configuration, with support for forcing postgres type and interactive region selection
func DefaultPostgres(ctx context.Context, plan *LaunchPlan, mpgEnabled bool) (PostgresPlan, error) {
	io := iostreams.FromContext(ctx)
	isInteractive := io != nil && io.IsInteractive()

	// Check the --db flag value to determine postgres type preference
	dbFlag := flag.GetString(ctx, "db")

	// Normalize db flag values
	var forceType string
	switch dbFlag {
	case "true", "1", "yes":
		forceType = "default" // Use default behavior
	case "mpg", "managed":
		forceType = "managed" // Force managed postgres
	case "upg", "unmanaged", "legacy":
		forceType = "unmanaged" // Force unmanaged postgres
	default:
		forceType = "default" // Default behavior for empty or unrecognized values
	}

	// If forced to use unmanaged postgres, use it regardless of mpgEnabled
	if forceType == "unmanaged" {
		return createFlyPostgresPlan(plan), nil
	}

	// If forced to use managed postgres, handle region availability
	if forceType == "managed" {
		return handleForcedManagedPostgres(ctx, plan)
	}

	// Normal flow: prefer managed if enabled and available
	if mpgEnabled {
		orgSlug, err := mpg.ResolveOrganizationSlug(ctx, plan.OrgSlug)
		if err != nil {
			return createFlyPostgresPlan(plan), nil
		}

		validRegion, err := mpg.IsValidMPGRegion(ctx, orgSlug, plan.RegionCode)
		if err == nil && validRegion {
			// Managed postgres is available in this region, use it
			return createManagedPostgresPlan(ctx, plan, "basic"), nil
		}

		// Managed postgres is not available in this region
		if isInteractive {
			// Offer to switch to a nearby region that supports managed postgres
			return handleInteractiveRegionSwitch(ctx, plan, orgSlug)
		} else {
			// Non-interactive: log warning and fall back to FlyPostgres
			if io != nil {
				fmt.Fprintf(io.ErrOut, "Warning: Using Unmanaged Postgres because Managed Postgres isn't yet available in region %s\n", plan.RegionCode)
			}
		}
	}

	// Default to FlyPostgres
	return createFlyPostgresPlan(plan), nil
}

// createFlyPostgresPlan creates a FlyPostgres plan with default values
func createFlyPostgresPlan(plan *LaunchPlan) PostgresPlan {
	return PostgresPlan{
		FlyPostgres: &FlyPostgresPlan{
			// NOTE: Until Legacy Launch is removed, we have to maintain
			//       "%app_name%-db" as the app name for the database.
			//       (legacy launch does not have a single source-of-truth name for the db,
			//        so it constructs the name on-the-spot each time it needs it)
			AppName:    plan.AppName + "-db",
			VmSize:     "shared-cpu-1x",
			VmRam:      256,
			Nodes:      1,
			DiskSizeGB: 1,
			Price:      -1,
		},
	}
}

// createManagedPostgresPlan creates a managed postgres plan and displays cost information
func createManagedPostgresPlan(ctx context.Context, plan *LaunchPlan, planType string) PostgresPlan {
	io := iostreams.FromContext(ctx)

	// Display plan details if we have an IO context
	if io != nil && planType != "" {
		if planDetails, exists := mpg.MPGPlans[planType]; exists {
			fmt.Fprintf(io.Out, "\nSelected Managed Postgres Plan: %s\n", planDetails.Name)
			fmt.Fprintf(io.Out, "  CPU: %s\n", planDetails.CPU)
			fmt.Fprintf(io.Out, "  Memory: %s\n", planDetails.Memory)
			fmt.Fprintf(io.Out, "  Price: $%d per month\n\n", planDetails.PricePerMo)
		}
	}

	return PostgresPlan{
		ManagedPostgres: &ManagedPostgresPlan{
			DbName:   plan.AppName + "-db",
			Region:   plan.RegionCode,
			Plan:     planType,
			DiskSize: 10, // Default managed postgres disk size
		},
	}
}

// handleForcedManagedPostgres handles the case where managed postgres is forced but may not be available
func handleForcedManagedPostgres(ctx context.Context, plan *LaunchPlan) (PostgresPlan, error) {
	io := iostreams.FromContext(ctx)

	orgSlug, err := mpg.ResolveOrganizationSlug(ctx, plan.OrgSlug)
	if err != nil {
		return createFlyPostgresPlan(plan), nil
	}

	validRegion, err := mpg.IsValidMPGRegion(ctx, orgSlug, plan.RegionCode)

	if err == nil && validRegion {
		// Region supports managed postgres
		return createManagedPostgresPlan(ctx, plan, "basic"), nil
	}

	// Region doesn't support managed postgres
	isInteractive := io != nil && io.IsInteractive()
	if isInteractive {
		// Interactive: suggest switching to a supported region
		return handleInteractiveRegionSwitch(ctx, plan, orgSlug)
	} else {
		// Non-interactive: fail with error
		availableCodes, _ := mpg.GetAvailableMPGRegionCodes(ctx, orgSlug)
		return PostgresPlan{}, fmt.Errorf("managed postgres is not available in region %s. Available regions: %v", plan.RegionCode, availableCodes)
	}
}

// handleInteractiveRegionSwitch prompts user to switch to a region that supports managed postgres
func handleInteractiveRegionSwitch(ctx context.Context, plan *LaunchPlan, orgSlug string) (PostgresPlan, error) {
	io := iostreams.FromContext(ctx)

	// Get available MPG regions
	availableRegions, err := mpg.GetAvailableMPGRegions(ctx, orgSlug)
	if err != nil || len(availableRegions) == 0 {
		if io != nil {
			fmt.Fprintf(io.ErrOut, "Warning: Unable to find regions that support Managed Postgres. Using Unmanaged Postgres in region %s\n", plan.RegionCode)
		}
		return createFlyPostgresPlan(plan), nil
	}

	// Ask user if they want to switch regions
	if io != nil {
		fmt.Fprintf(io.Out, "Managed Postgres is not available in region %s.\n", plan.RegionCode)
	}

	confirmed, err := prompt.Confirm(ctx, "Would you like to switch to a region that supports Managed Postgres?")
	if err != nil || !confirmed {
		if io != nil {
			fmt.Fprintf(io.ErrOut, "Using Unmanaged Postgres in region %s\n", plan.RegionCode)
		}
		return createFlyPostgresPlan(plan), nil
	}

	// Present region options
	var regionOptions []string
	for _, region := range availableRegions {
		regionOptions = append(regionOptions, fmt.Sprintf("%s (%s)", region.Name, region.Code))
	}

	var selectedIndex int
	if err := prompt.Select(ctx, &selectedIndex, "Select a region for Managed Postgres", "", regionOptions...); err != nil {
		if io != nil {
			fmt.Fprintf(io.ErrOut, "Failed to select region. Using Unmanaged Postgres in region %s\n", plan.RegionCode)
		}
		return createFlyPostgresPlan(plan), nil
	}

	// Update the plan with the new region - this changes the overall app region,
	// not just the postgres region, so the entire app launches in the MPG-supported region
	selectedRegion := availableRegions[selectedIndex]
	plan.RegionCode = selectedRegion.Code

	if io != nil {
		fmt.Fprintf(io.Out, "Switched to region %s (%s) for Managed Postgres support.\nYour app will now launch in this region.\n", selectedRegion.Name, selectedRegion.Code)
	}

	return createManagedPostgresPlan(ctx, plan, "basic"), nil
}

type FlyPostgresPlan struct {
	AppName    string `json:"app_name"`
	VmSize     string `json:"vm_size"`
	VmRam      int    `json:"vm_ram"`
	Nodes      int    `json:"nodes"`
	DiskSizeGB int    `json:"disk_size_gb"`
	AutoStop   bool   `json:"auto_stop"`
	Price      int    `json:"price"`
}

func (p *FlyPostgresPlan) Guest() *fly.MachineGuest {
	guest := fly.MachineGuest{}
	guest.SetSize(p.VmSize)
	if p.VmRam != 0 {
		guest.MemoryMB = p.VmRam
	}
	return &guest
}

type SupabasePostgresPlan struct {
	DbName string `json:"db_name"`
	Region string `json:"region"`
}

func (p *SupabasePostgresPlan) GetDbName(plan *LaunchPlan) string {
	if p.DbName == "" {
		return plan.AppName + "-db"
	}
	return p.DbName
}

func (p *SupabasePostgresPlan) GetRegion(plan *LaunchPlan) string {
	if p.Region == "" {
		return plan.RegionCode
	}
	return p.Region
}

type ManagedPostgresPlan struct {
	DbName    string `json:"db_name"`
	Region    string `json:"region"`
	Plan      string `json:"plan"`
	DiskSize  int    `json:"disk_size"`
	ClusterID string `json:"cluster_id,omitempty"`
}

func (p *ManagedPostgresPlan) GetDbName(plan *LaunchPlan) string {
	if p.DbName == "" {
		return plan.AppName + "-db"
	}
	return p.DbName
}

func (p *ManagedPostgresPlan) GetRegion(plan *LaunchPlan) string {
	if p.Region == "" {
		return plan.RegionCode
	}
	return p.Region
}
