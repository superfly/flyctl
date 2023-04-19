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
			Description: "Specify an existing application name to seed from. A specific volume can be specified with the format: <app-name>:<vol-id>",
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

	var region *api.Region

	region, err = prompt.Region(ctx, !org.PaidPlan, prompt.RegionParams{
		Message: "",
	})
	if err != nil {
		return
	}

	pgConfig := PostgresConfiguration{
		Name:               appName,
		VMSize:             flag.GetString(ctx, "vm-size"),
		InitialClusterSize: flag.GetInt(ctx, "initial-cluster-size"),
		ImageRef:           flag.GetString(ctx, "image-ref"),
		DiskGb:             flag.GetInt(ctx, "volume-size"),
	}

	params := &ClusterParams{
		PostgresConfiguration: pgConfig,
		Password:              flag.GetString(ctx, "password"),
		SnapshotID:            flag.GetString(ctx, "snapshot-id"),
		Detach:                flag.GetDetach(ctx),
		Manager:               flypg.StolonManager,
		Autostart:             flag.GetBool(ctx, "autostart"),
	}

	if flag.GetString(ctx, "fork-from") != "" {
		if params.SnapshotID != "" {
			return fmt.Errorf("cannot specify both --fork-from and --snapshot-id")
		}

		if params.PostgresConfiguration.InitialClusterSize > 1 {
			fmt.Fprintf(io.Out, colorize.Yellow("Warning: --initial-cluster-size is ignored when forking from an existing cluster\n"))
			params.PostgresConfiguration.InitialClusterSize = 1
		}

		vol, err := resolveForkFromTarget(ctx, client, flag.GetString(ctx, "fork-from"))
		if err != nil {
			return err
		}

		if vol == nil {
			return fmt.Errorf("Unable to resolve volume from fork-from target: %s", flag.GetString(ctx, "fork-from"))
		}

		if vol.Region != region.Code {
			return fmt.Errorf("The target region %q must match the region associated with the fork target: %q", region.Code, vol.Region)
		}

		fmt.Fprintf(io.Out, "Forking from volume %s\n", vol.ID)

		params.ForkFrom = vol.ID
	}

	params.Manager = flypg.ReplicationManager
	if flag.GetBool(ctx, "stolon") {
		params.Manager = flypg.StolonManager
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
		ImageRef:     params.ImageRef,
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
		var selected int

		options := []string{}
		for _, cfg := range postgresConfigurations(input.Manager) {
			options = append(options, cfg.Description)
		}

		if err := prompt.Select(ctx, &selected, msg, "", options...); err != nil {
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
		if params.InitialClusterSize == 0 {
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
		input.InitialClusterSize = params.InitialClusterSize

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

func resolveForkFromTarget(ctx context.Context, client *api.Client, forkFrom string) (*api.Volume, error) {
	var app *api.AppCompact
	var volume *api.Volume
	var err error

	// Check to see whether the fork-from value includes a volume ID
	forkSlice := strings.Split(forkFrom, ":")

	if len(forkSlice) > 2 {
		return nil, fmt.Errorf("invalid --fork-from format")
	}

	app, err = client.GetAppCompact(ctx, forkSlice[0])
	if err != nil {
		return nil, fmt.Errorf("failed to resolve from-from app %s: %w", forkSlice[0], err)
	}

	if len(forkSlice) == 2 {
		volume, err = client.GetVolume(ctx, forkSlice[1])
		if err != nil {
			return nil, err
		}
	}

	if !app.IsPostgresApp() {
		return nil, fmt.Errorf("app %s is not a postgres app", app.Name)
	}

	// If volume was specified, we are done here
	if volume != nil {
		return volume, nil
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return nil, err
	}

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list machines on target app %s: %w", app.Name, err)
	}

	if len(machines) == 0 {
		return nil, fmt.Errorf("No machines associated with %s, please manually specify the volume you wish to fork. See `fly pg create --help` for more information", app.Name)
	}

	primaryRegion := machines[0].Config.Env["PRIMARY_REGION"]

	for _, m := range machines {
		// Exclude machines that are not in the primary region
		if m.Config.Env["PRIMARY_REGION"] != primaryRegion {
			continue
		}

		role := machineRole(m)
		// Exclude machines that are not primary (flex) or leader (stolon)
		if role != "primary" && role != "leader" {
			continue
		}

		volID := m.Config.Mounts[0].Volume
		vol, err := client.GetVolume(ctx, volID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the volume associated with the leader %s: %w", volID, err)
		}

		return vol, nil
	}

	return nil, err
}
