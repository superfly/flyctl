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
			Name:        "machines",
			Description: "Create postgres cluster on fly machines",
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
		appName  = flag.GetString(ctx, "name")
		imageRef = flag.GetString(ctx, "image-ref")
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

	region, err = prompt.Region(ctx, "")
	if err != nil {
		return
	}

	input := &flypg.CreateClusterInput{
		AppName:      appName,
		Organization: org,
		ImageRef:     imageRef,
		Region:       region.Code,
	}

	volumeSize := flag.GetInt(ctx, "volume-size")
	initialClusterSize := flag.GetInt(ctx, "initial-cluster-size")
	vmSizeName := flag.GetString(ctx, "vm-size")

	customConfig := volumeSize != 0 || vmSizeName != "" || initialClusterSize != 0

	var config *PostgresConfiguration

	if !customConfig {
		fmt.Fprintf(io.Out, "For pricing information visit: https://fly.io/docs/about/pricing/#postgresql-clusters")

		var msg = "Select configuration:"

		var selected int

		options := []string{}
		for _, cfg := range postgresConfigurations() {
			options = append(options, cfg.Description)
		}

		if err := prompt.Select(ctx, &selected, msg, "", options...); err != nil {
			return err
		}
		config = &postgresConfigurations()[selected]

		if config.VmSize == "" {
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
		vmSize, err := prompt.VMSize(ctx, vmSizeName)
		if err != nil {
			return err
		}

		input.VMSize = api.StringPointer(vmSize.Name)

		// Resolve volume size
		if volumeSize == 0 {
			if err = prompt.Int(ctx, &volumeSize, "Volume size", 10, false); err != nil {
				return err
			}
		}
		input.VolumeSize = api.IntPointer(volumeSize)
	} else {
		// Resolve configuration from pre-defined configuration.
		vmSize, err := prompt.VMSize(ctx, config.VmSize)
		if err != nil {
			return err
		}

		input.VMSize = api.StringPointer(vmSize.Name)
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
		input.SnapshotID = snapshot
	}

	fmt.Fprintf(io.Out, "Creating postgres cluster %s in organization %s\n", appName, org.Slug)

	launcher := flypg.NewLauncher(client)

	if flag.GetBool(ctx, "machines") {
		return launcher.LaunchMachinesPostgres(ctx, input)
	}
	return launcher.LaunchNomadPostgres(ctx, input)
}

type PostgresConfiguration struct {
	Name               string
	Description        string
	ImageRef           string
	InitialClusterSize int
	VmSize             string
	MemoryMb           int
	DiskGb             int
}

func postgresConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:        "Development - Single node, 1x shared CPU, 256MB RAM, 1GB disk",
			DiskGb:             1,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 1,
			MemoryMb:           256,
			VmSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x shared CPU, 256MB RAM, 10GB disk",
			DiskGb:             10,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           256,
			VmSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x Dedicated CPU, 2GB RAM, 50GB disk",
			DiskGb:             50,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           2048,
			VmSize:             "dedicated-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 2x Dedicated CPU's, 4GB RAM, 100GB disk",
			DiskGb:             100,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           4096,
			VmSize:             "dedicated-cpu-2x",
		},
		{
			Description:        "Specify custom configuration",
			DiskGb:             0,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 0,
			MemoryMb:           0,
			VmSize:             "",
		},
	}
}
