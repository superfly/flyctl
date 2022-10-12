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

	cmd := command.New("create", short, long, runCreate,
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
			Name:    "image-ref",
			Hidden:  true,
			Default: "flyio/postgres",
		},
		flag.Bool{
			Name:        "nomad",
			Description: "Create postgres cluster on Nomad",
			Default:     false,
		},
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)
	var (
		appName            = flag.GetString(ctx, "name")
		imageRef           = flag.GetString(ctx, "image-ref")
		volumeSize         = flag.GetInt(ctx, "volume-size")
		initialClusterSize = flag.GetInt(ctx, "initial-cluster-size")
		vmSizeName         = flag.GetString(ctx, "vm-size")
		targetPlatform     = resolveTargetPlatform(ctx)
	)

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

	input := &flypg.CreateClusterInput{
		AppName:      appName,
		Organization: org,
		ImageRef:     imageRef,
		Region:       region.Code,
	}

	customConfig := volumeSize != 0 || vmSizeName != "" || initialClusterSize != 0

	var config *PostgresConfiguration

	if !customConfig {
		fmt.Fprintf(io.Out, "For pricing information visit: https://fly.io/docs/about/pricing/#postgresql-clusters")

		msg := "Select configuration:"

		var selected int

		options := []string{}
		for _, cfg := range postgresConfigurations(targetPlatform) {
			options = append(options, cfg.Description)
		}

		if err := prompt.Select(ctx, &selected, msg, "", options...); err != nil {
			return err
		}
		config = &postgresConfigurations(targetPlatform)[selected]

		if config.VMSize == "" {
			// User has opted into choosing a custom configuration.
			customConfig = true
		}
	}

	if customConfig {
		// Resolve cluster size
		if initialClusterSize == 0 {
			err := prompt.Int(ctx, &initialClusterSize, "Initial cluster size", 2, true)
			if err != nil {
				return err
			}
		}
		input.InitialClusterSize = initialClusterSize

		// Resolve VM size
		vmSize, err := resolveVMSize(ctx, targetPlatform, vmSizeName)
		if err != nil {
			return err
		}

		input.VMSize = vmSize

		// Resolve volume size
		if volumeSize == 0 {
			if err = prompt.Int(ctx, &volumeSize, "Volume size", 10, false); err != nil {
				return err
			}
		}
		input.VolumeSize = api.IntPointer(volumeSize)
	} else {
		// Resolve configuration from pre-defined configuration.
		vmSize, err := resolveVMSize(ctx, targetPlatform, config.VMSize)
		if err != nil {
			return err
		}

		input.VMSize = vmSize
		input.VolumeSize = api.IntPointer(config.DiskGb)

		input.InitialClusterSize = config.InitialClusterSize

		if imageRef := flag.GetString(ctx, "image-ref"); imageRef != "" {
			input.ImageRef = imageRef
		} else {
			input.ImageRef = config.ImageRef
		}
	}

	if password := flag.GetString(ctx, "password"); password != "" {
		input.Password = password
	}

	snapshot := flag.GetString(ctx, "snapshot-id")
	if snapshot != "" {
		input.SnapshotID = &snapshot
	}

	fmt.Fprintf(io.Out, "Creating postgres cluster %s in organization %s\n", appName, org.Slug)

	launcher := flypg.NewLauncher(client)

	if flag.GetBool(ctx, "nomad") {
		return launcher.LaunchNomadPostgres(ctx, input)
	}
	return launcher.LaunchMachinesPostgres(ctx, input)
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

func resolveTargetPlatform(ctx context.Context) string {
	if flag.GetBool(ctx, "nomad") {
		return "nomad"
	}

	return "machines"
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
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 1,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 2x shared CPUs, 4GB RAM, 40GB disk",
			DiskGb:             40,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           4096,
			VMSize:             "shared-cpu-2x",
		},
		{
			Description:        "Production - Highly available, 4x shared CPUs, 8GB RAM, 80GB disk",
			DiskGb:             80,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           8192,
			VMSize:             "shared-cpu-4x",
		},
		{
			Description:        "Specify custom configuration",
			DiskGb:             0,
			ImageRef:           "flyio/postgres",
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
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 1,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x shared CPU, 256MB RAM, 10GB disk",
			DiskGb:             10,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           256,
			VMSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x Dedicated CPU, 2GB RAM, 50GB disk",
			DiskGb:             50,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           2048,
			VMSize:             "dedicated-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 2x Dedicated CPUs, 4GB RAM, 100GB disk",
			DiskGb:             100,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           4096,
			VMSize:             "dedicated-cpu-2x",
		},
		{
			Description:        "Specify custom configuration",
			DiskGb:             0,
			ImageRef:           "flyio/postgres",
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
