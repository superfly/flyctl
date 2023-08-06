package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a new PostgreSQL cluster"
		long  = short + "\n"
	)

	cmd := command.New("create", short, long, run,
		command.RequireSession,
	)

	flag.Add(
		cmd,
		flag.Region(),
		flag.Org(),
		flag.Detach(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Postgres app",
		},
		flag.String{
			Name:        "password",
			Shorthand:   "p",
			Description: "The superuser password. The password will be generated for you if you leave this blank",
		},
		flag.String{
			Name:        "vm-size",
			Description: "the size of the VM",
		},

		flag.Int{
			Name:        "initial-cluster-size",
			Description: "Initial cluster size",
		},
		flag.Int{
			Name:        "volume-size",
			Description: "The volume size in GB",
		},
		flag.String{
			Name:        "consul-url",
			Description: "Opt into using an existing consul as the backend store by specifying the target consul url.",
		},
		flag.String{
			Name:        "snapshot-id",
			Description: "Creates the volume with the contents of the snapshot",
		},
		flag.String{
			Name:        "fork-from",
			Description: "Specify a source Postgres application to fork from. Format: <app-name> or <app-name>:<volume-id>",
		},
		flag.String{
			Name:        "image-ref",
			Description: "Specify a non-default base image for the Postgres app",
		},
		flag.Bool{
			Name:        "stolon",
			Description: "Create a postgres cluster that's managed by Stolon",
			Default:     false,
		},
		flag.Bool{
			Name:        "flex",
			Description: "Create a postgres cluster that's managed by Repmgr",
			Default:     true,
		},
		flag.Bool{
			Name:        "autostart",
			Description: "Automatically start a stopped Postgres app when a network request is received",
			Default:     false,
		},
	)

	return cmd
}

// Since we want to be able to create PG clusters from other commands,
// we pass the name, region and org to CreateCluster. Other flags that don't prompt may
// be safely passed through from other commands.
func run(ctx context.Context) (err error) {
	var (
		appName  = flag.GetString(ctx, "name")
		client   = client.FromContext(ctx).API()
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	// pre-fetch platform regions for later use
	prompt.PlatformRegions(ctx)

	if appName == "" {
		if appName, err = prompt.SelectAppName(ctx); err != nil {
			return
		}
	}

	var org *api.Organization
	org, err = prompt.Org(ctx)
	if err != nil {
		return
	}

	params := &ClusterParams{
		Password:   flag.GetString(ctx, "password"),
		SnapshotID: flag.GetString(ctx, "snapshot-id"),
		Detach:     flag.GetDetach(ctx),
		Autostart:  flag.GetBool(ctx, "autostart"),
	}

	pgConfig := &PostgresConfiguration{
		Name:               appName,
		VMSize:             flag.GetString(ctx, "vm-size"),
		InitialClusterSize: flag.GetInt(ctx, "initial-cluster-size"),
		ImageRef:           flag.GetString(ctx, "image-ref"),
		DiskGb:             flag.GetInt(ctx, "volume-size"),
	}

	forkFrom := flag.GetString(ctx, "fork-from")

	if forkFrom != "" {
		// Snapshot ID may not be specified with fork-from
		if params.SnapshotID != "" {
			return fmt.Errorf("cannot specify both --fork-from and --snapshot-id")
		}

		// If initial-cluster-size is not specified, set it to 1
		if pgConfig.InitialClusterSize == 0 {
			pgConfig.InitialClusterSize = 1
		}

		// Initial cluster size may not be greater than 1 with fork-from
		if pgConfig.InitialClusterSize > 1 {
			fmt.Fprintf(io.Out, colorize.Yellow("Warning: --initial-cluster-size is ignored when specifying --fork-from\n"))
			pgConfig.InitialClusterSize = 1
		}

		// Check to see whether the fork-from value includes a volume ID
		forkSlice := strings.Split(forkFrom, ":")
		if len(forkSlice) > 2 {
			return fmt.Errorf("invalid --fork-from format. See `fly pg create --help` for more information")
		}

		// Resolve specified fork-from app
		forkApp, err := client.GetAppCompact(ctx, forkSlice[0])
		if err != nil {
			return fmt.Errorf("Failed to resolve the specified fork-from app %s: %w", forkSlice[0], err)
		}

		// Confirm fork-app is a postgres app
		if !forkApp.IsPostgresApp() {
			return fmt.Errorf("The fork-from app %q must be a postgres app", forkApp.Name)
		}

		ctx, err := apps.BuildContext(ctx, forkApp)
		if err != nil {
			return err
		}

		machines, err := mach.ListActive(ctx)
		if err != nil {
			return fmt.Errorf("Failed to list machines associated with %s: %w", forkApp.Name, err)
		}

		// If the fork-from value includes a volume ID, use that. Otherwise, attempt to resolve the
		// volume ID associated with the fork-from app's primary instance.
		if len(forkSlice) == 2 {
			params.ForkFrom = forkSlice[1]
		} else {
			volID, err := resolveForkFromVolume(ctx, machines)
			if err != nil {
				return err
			}

			params.ForkFrom = volID
		}

		// Resolve the volume
		vol, err := client.GetVolume(ctx, params.ForkFrom)
		if err != nil {
			return fmt.Errorf("Failed to resolve the specified fork-from volume %s: %w", params.ForkFrom, err)
		}

		// Confirm that the volume is associated with the fork-from app
		if vol.App.Name != forkApp.Name {
			return fmt.Errorf("The volume %q specified must be associated with the fork-from app %q", vol.ID, forkApp.Name)
		}

		// If the region isn't specified, set the region of the fork target
		reg := flag.GetString(ctx, "region")
		if reg == "" {
			reg = vol.Region
			flag.SetString(ctx, "region", vol.Region)
		}

		// Confirm that the region matches the region of the fork target
		if reg != "" && reg != vol.Region {
			return fmt.Errorf("Target region %q must match the region of the fork volume target: %q", reg, vol.Region)
		}

		// If the volume size isn't specified, set the volume size of the fork target
		if pgConfig.DiskGb == 0 {
			pgConfig.DiskGb = vol.SizeGb
		}

		// Confirm that the volume size is greater than or equal to the fork target
		if pgConfig.DiskGb < vol.SizeGb {
			return fmt.Errorf("The target volume size %dGB must be greater than or equal to the volume fork target: %dGB", pgConfig.DiskGb, vol.SizeGb)
		}

		// Attempt to resolve the image ref from the machine tied to the fork volume.
		if pgConfig.ImageRef == "" {
			pgConfig.ImageRef = resolveImageFromForkVolume(vol, machines)
		}

		// Resolve the fork-from app manager
		params.Manager = resolveForkFromManager(ctx, machines)
		params.ForkFrom = vol.ID
	}

	params.PostgresConfiguration = *pgConfig

	var region *api.Region
	region, err = prompt.Region(ctx, !org.PaidPlan, prompt.RegionParams{
		Message: "",
	})
	if err != nil {
		return
	}

	// Set the default manager if not specified or already resolved
	if params.Manager == "" {
		params.Manager = flypg.ReplicationManager
		if flag.GetBool(ctx, "stolon") {
			params.Manager = flypg.StolonManager
		}
	}

	return CreateCluster(ctx, org, region, params)
}

// CreateCluster creates a Postgres cluster with an optional name. The name will be prompted for if not supplied.
func CreateCluster(ctx context.Context, org *api.Organization, region *api.Region, params *ClusterParams) (err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	input := &flypg.CreateClusterInput{
		AppName:      params.Name,
		Organization: org,
		ImageRef:     params.PostgresConfiguration.ImageRef,
		Region:       region.Code,
		Manager:      params.Manager,
		Autostart:    params.Autostart,
		ForkFrom:     params.ForkFrom,
	}

	customConfig := params.DiskGb != 0 || params.VMSize != "" || params.InitialClusterSize != 0

	var config *PostgresConfiguration

	if !customConfig {
		fmt.Fprintf(io.Out, "For pricing information visit: https://fly.io/docs/about/pricing/#postgresql-clusters")

		msg := "Select configuration:"
		configurations := postgresConfigurations(input.Manager)
		var selected int

		options := []string{}
		for i, cfg := range configurations {
			options = append(options, cfg.Description)
			if selected == 0 && !strings.HasPrefix(cfg.Description, "Dev") {
				selected = i
			}
		}

		if err := prompt.Select(ctx, &selected, msg, configurations[selected].Description, options...); err != nil {
			return err
		}
		config = &postgresConfigurations(input.Manager)[selected]

		if input.Manager == flypg.ReplicationManager && config.VMSize == "shared-cpu-1x" {
			confirm, err := prompt.Confirm(ctx, "Scale single node pg to zero after one hour?")
			if err != nil {
				return err
			}
			input.ScaleToZero = confirm
		}

		if config.VMSize == "" {
			// User has opted into choosing a custom configuration.
			customConfig = true
		}
	}

	if customConfig {
		// Resolve cluster size
		if params.PostgresConfiguration.InitialClusterSize == 0 {
			clusterSizePrompt := "Initial cluster size"
			defaultClusterSize := 2

			if input.Manager == flypg.ReplicationManager {
				defaultClusterSize = 3
				clusterSizePrompt = "Initial cluster size - Specify at least 3 for HA"
			}

			err := prompt.Int(ctx, &params.InitialClusterSize, clusterSizePrompt, defaultClusterSize, true)
			if err != nil {
				return err
			}
		}
		input.InitialClusterSize = params.PostgresConfiguration.InitialClusterSize

		// Resolve VM size
		vmSize, err := resolveVMSize(ctx, params.VMSize)
		if err != nil {
			return err
		}

		input.VMSize = vmSize

		// Resolve volume size
		if params.DiskGb == 0 {
			if err = prompt.Int(ctx, &params.DiskGb, "Volume size", 10, false); err != nil {
				return err
			}
		}
		input.VolumeSize = api.IntPointer(params.DiskGb)
		input.Autostart = params.Autostart
	} else {
		// Resolve configuration from pre-defined configuration.
		vmSize, err := resolveVMSize(ctx, config.VMSize)
		if err != nil {
			return err
		}

		input.VMSize = vmSize
		input.VolumeSize = api.IntPointer(config.DiskGb)
		input.InitialClusterSize = config.InitialClusterSize
		input.ImageRef = params.ImageRef
		input.Autostart = params.Autostart
	}

	if params.Password != "" {
		input.Password = params.Password
	}

	if params.SnapshotID != "" {
		input.SnapshotID = &params.SnapshotID
	}

	fmt.Fprintf(io.Out, "Creating postgres cluster in organization %s\n", org.Slug)

	launcher := flypg.NewLauncher(client)

	return launcher.LaunchMachinesPostgres(ctx, input, params.Detach)
}

func resolveVMSize(ctx context.Context, targetSize string) (*api.VMSize, error) {
	// verify the specified size
	if targetSize != "" {
		for _, size := range MachineVMSizes() {
			if targetSize == size.Name {
				return &size, nil
			}
		}

		return nil, fmt.Errorf("vm size %q is not valid", targetSize)
	}
	// prompt user to select machine specific size.
	return prompt.SelectVMSize(ctx, MachineVMSizes())
}

type PostgresConfiguration struct {
	Name               string
	Description        string
	ImageRef           string
	InitialClusterSize int
	VMSize             string
	MemoryMb           int
	DiskGb             int
}

type ClusterParams struct {
	PostgresConfiguration
	Password   string
	SnapshotID string
	Detach     bool
	Manager    string
	ForkFrom   string
	Autostart  bool
}

func postgresConfigurations(manager string) []PostgresConfiguration {
	if manager == flypg.StolonManager {
		return stolonConfigurations()
	}
	return flexConfigurations()
}

func stolonConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:        "Development - Single node, 1x shared CPU, 256MB RAM, 1GB disk",
			DiskGb:             1,
			InitialClusterSize: 1,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 2x shared CPUs, 4GB RAM, 40GB disk",
			DiskGb:             40,
			InitialClusterSize: 2,
			MemoryMb:           4096,
			VMSize:             "shared-cpu-2x",
		},
		{
			Description:        "Production - Highly available, 4x shared CPUs, 8GB RAM, 80GB disk",
			DiskGb:             80,
			InitialClusterSize: 2,
			MemoryMb:           8192,
			VMSize:             "shared-cpu-4x",
		},
		{
			Description:        "Specify custom configuration",
			DiskGb:             0,
			InitialClusterSize: 0,
			MemoryMb:           0,
			VMSize:             "",
		},
	}
}

func flexConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:        "Development - Single node, 1x shared CPU, 256MB RAM, 1GB disk",
			DiskGb:             1,
			InitialClusterSize: 1,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production (High Availability) - 3 nodes, 2x shared CPUs, 4GB RAM, 40GB disk",
			DiskGb:             40,
			InitialClusterSize: 3,
			MemoryMb:           4096,
			VMSize:             "shared-cpu-2x",
		},
		{
			Description:        "Production (High Availability) - 3 nodes, 4x shared CPUs, 8GB RAM, 80GB disk",
			DiskGb:             80,
			InitialClusterSize: 3,
			MemoryMb:           8192,
			VMSize:             "shared-cpu-4x",
		},
		{
			Description:        "Specify custom configuration",
			DiskGb:             0,
			InitialClusterSize: 0,
			MemoryMb:           0,
			VMSize:             "",
		},
	}
}

// machineVMSizes represents the available VM configurations for Machines.
func MachineVMSizes() []api.VMSize {
	// TODO - Eventually we will have a flaps endpoint for this.
	return []api.VMSize{
		{
			Name:     "shared-cpu-1x",
			CPUClass: "shared",
			CPUCores: 1,
			MemoryMB: 256,
			MemoryGB: 0.25,
		},
		{
			Name:     "shared-cpu-2x",
			CPUClass: "shared",
			CPUCores: 2,
			MemoryMB: 4096,
			MemoryGB: 4,
		},
		{
			Name:     "shared-cpu-4x",
			CPUClass: "shared",
			CPUCores: 4,
			MemoryMB: 8192,
			MemoryGB: 8,
		},
		{
			Name:     "shared-cpu-8x",
			CPUClass: "shared",
			CPUCores: 8,
			MemoryMB: 16384,
			MemoryGB: 16,
		},
		{
			Name:     "performance-1x",
			CPUClass: "performance",
			CPUCores: 1,
			MemoryMB: 2048,
			MemoryGB: 2,
		},
		{
			Name:     "performance-2x",
			CPUClass: "performance",
			CPUCores: 2,
			MemoryMB: 4096,
			MemoryGB: 4,
		},
		{
			Name:     "performance-4x",
			CPUClass: "performance",
			CPUCores: 4,
			MemoryMB: 8192,
			MemoryGB: 8,
		},
		{
			Name:     "performance-8x",
			CPUClass: "performance",
			CPUCores: 8,
			MemoryMB: 16384,
			MemoryGB: 16,
		},
		{
			Name:     "performance-16x",
			CPUClass: "performance",
			CPUCores: 16,
			MemoryMB: 32768,
			MemoryGB: 32,
		},
	}
}

func resolveForkFromVolume(ctx context.Context, machines []*api.Machine) (string, error) {
	if len(machines) == 0 {
		return "", fmt.Errorf("No machines associated with fork-from target. See `fly pg create --help` for more information")
	}

	primaryRegion := machines[0].Config.Env["PRIMARY_REGION"]

	// Attempt to resolve the volume associated with the primary machine
	for _, m := range machines {
		// Exclude machines that are not in the primary region
		if m.Config.Env["PRIMARY_REGION"] != primaryRegion {
			continue
		}

		role := machineRole(m)
		// Exclude machines that are not the primary (flex) or leader (stolon)
		if role != "primary" && role != "leader" {
			continue
		}

		return m.Config.Mounts[0].Volume, nil
	}

	return "", fmt.Errorf("Failed to resolve the volume associated with the primary instance. See `fly pg create --help` for more information")
}

func resolveForkFromManager(ctx context.Context, machines []*api.Machine) string {
	if flag.GetBool(ctx, "stolon") {
		return flypg.StolonManager
	}

	// We can't resolve the manager type, so we'll default to repmgr
	if len(machines) == 0 {
		return flypg.ReplicationManager
	}

	if machines[0].ImageRef.Labels["fly.pg-manager"] == flypg.ReplicationManager {
		return flypg.ReplicationManager
	}

	return flypg.StolonManager
}

func resolveImageFromForkVolume(vol *api.Volume, machines []*api.Machine) string {
	// Attempt to resolve the machine image that's associated with the volume
	for _, m := range machines {
		if m.Config.Mounts[0].Volume == vol.ID {
			return m.FullImageRef()
		}
	}

	return ""
}
