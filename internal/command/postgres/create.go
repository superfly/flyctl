package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
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
			Name:        "machines",
			Description: "Create postgres cluster on fly machines",
			Default:     true,
		},
		flag.Bool{
			Name:        "nomad",
			Description: "Create postgres cluster on Nomad",
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
		if appName, err = apps.SelectAppName(ctx); err != nil {
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

	platform := "machines"

	if flag.GetBool(ctx, "nomad") {
		platform = "nomad"
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
	}
	return CreateCluster(ctx, org, region, platform, params)
}

// CreateCluster creates a Postgres cluster with an optional name. The name will be prompted for if not supplied.
func CreateCluster(ctx context.Context, org *api.Organization, region *api.Region, platform string, params *ClusterParams) (err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	input := &flypg.CreateClusterInput{
		AppName:      params.Name,
		Organization: org,
		ImageRef:     params.ImageRef,
		Region:       region.Code,
	}

	customConfig := params.DiskGb != 0 || params.VMSize != "" || params.InitialClusterSize != 0

	var config *PostgresConfiguration

	if !customConfig {
		fmt.Fprintf(io.Out, "For pricing information visit: https://fly.io/docs/about/pricing/#postgresql-clusters")

		msg := "Select configuration:"

		var selected int

		options := []string{}
		for _, cfg := range postgresConfigurations(platform) {
			options = append(options, cfg.Description)
		}

		if err := prompt.Select(ctx, &selected, msg, "", options...); err != nil {
			return err
		}
		config = &postgresConfigurations(platform)[selected]

		if config.VMSize == "" {
			// User has opted into choosing a custom configuration.
			customConfig = true
		}
	}

	if customConfig {
		// Resolve cluster size
		if params.InitialClusterSize == 0 {
			err := prompt.Int(ctx, &params.InitialClusterSize, "Initial cluster size", 2, true)
			if err != nil {
				return err
			}
		}
		input.InitialClusterSize = params.InitialClusterSize

		// Resolve VM size
		vmSize, err := resolveVMSize(ctx, platform, params.VMSize)
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
		vmSize, err := resolveVMSize(ctx, platform, config.VMSize)
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

	if platform == "nomad" {
		return launcher.LaunchNomadPostgres(ctx, input, params.Detach)
	}
	return launcher.LaunchMachinesPostgres(ctx, input, params.Detach)
}

func resolveVMSize(ctx context.Context, platform string, targetSize string) (*api.VMSize, error) {
	if platform == "machines" {
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

	return prompt.VMSize(ctx, targetSize)
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
}

func postgresConfigurations(platform string) []PostgresConfiguration {
	switch platform {
	case "machines":
		return postgresMachineConfigurations()
	default:
		return postgresNomadConfigurations()
	}
}

// postgresMachineConfiguration represents our pre-defined configurations for our Machine platform.
func postgresMachineConfigurations() []PostgresConfiguration {
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

// postgresConfiguration represents our pre-defined configurations for our Nomad platform.
func postgresNomadConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:        "Development - Single node, 1x shared CPU, 256MB RAM, 1GB disk",
			DiskGb:             1,
			InitialClusterSize: 1,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x shared CPU, 256MB RAM, 10GB disk",
			DiskGb:             10,
			InitialClusterSize: 2,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x Dedicated CPU, 2GB RAM, 50GB disk",
			DiskGb:             50,
			InitialClusterSize: 2,
			MemoryMb:           2048,
			VMSize:             "dedicated-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 2x Dedicated CPUs, 4GB RAM, 100GB disk",
			DiskGb:             100,
			InitialClusterSize: 2,
			MemoryMb:           4096,
			VMSize:             "dedicated-cpu-2x",
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
