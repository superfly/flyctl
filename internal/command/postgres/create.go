package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
			Name:   "image-ref",
			Hidden: true,
		},
		flag.Bool{
			Name:        "stolon",
			Description: "Create a postgres cluster that's managed by Stolon",
			Default:     true,
		},
		flag.Bool{
			Name:        "flex",
			Description: "Create a postgres cluster that's managed by Repmgr",
			Default:     false,
		},
	)

	return cmd
}

// Since we want to be able to create PG clusters from other commands,
// we pass the name, region and org to CreateCluster. Other flags that don't prompt may
// be safely passed through from other commands.
func run(ctx context.Context) (err error) {
	appName := flag.GetString(ctx, "name")

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

	region, err = prompt.Region(ctx, prompt.RegionParams{
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
	}

	params.Manager = flypg.StolonManager
	if flag.GetBool(ctx, "flex") {
		params.Manager = flypg.ReplicationManager
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

		if config.VMSize == "" {
			// User has opted into choosing a custom configuration.
			customConfig = true
		}
	}

	if customConfig {
		// Resolve cluster size
		if params.InitialClusterSize == 0 {
			defaultClusterSize := 3
			clusterSizePrompt := "Initial cluster size - Specify at least 3 for HA"

			if input.Manager == flypg.StolonManager {
				clusterSizePrompt = "Initial cluster size"
				defaultClusterSize = 2
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
			Description:        "Production - Highly available, 2x shared CPUs, 4GB RAM, 40GB disk",
			DiskGb:             40,
			InitialClusterSize: 3,
			MemoryMb:           4096,
			VMSize:             "shared-cpu-2x",
		},
		{
			Description:        "Production - Highly available, 4x shared CPUs, 8GB RAM, 80GB disk",
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
	}
}
