package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

var (
	volumeName          = "barman_data"
	volumePath          = "/data"
	Duration10s, _      = time.ParseDuration("10s")
	Duration15s, _      = time.ParseDuration("15s")
	CheckPathConnection = "/flycheck/connection"
	CheckPathRole       = "/flycheck/role"
	CheckPathVm         = "/flycheck/vm"
)

func newBarman() *cobra.Command {
	const (
		short = "Manage databases in a cluster"
		long  = short + "\n"
	)

	cmd := command.New("barman", short, long, nil)

	cmd.AddCommand(
		newCreateBarman(),
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func newCreateBarman() *cobra.Command {
	const (
		short = "create barman machine"
		long  = short + "\n"

		usage = "create"
	)

	cmd := command.New(usage, short, long, runBarmanCreate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.Region(),
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "vm-size",
			Description: "the size of the VM",
		},
		flag.Int{
			Name:        "volume-size",
			Description: "The volume size in GB",
		},
	)

	return cmd
}

func runBarmanCreate(ctx context.Context) error {
	var (
		io      = iostreams.FromContext(ctx)
		client  = client.FromContext(ctx).API()
		appName = appconfig.NameFromContext(ctx)
	)

	// pre-fetch platform regions for later use
	prompt.PlatformRegions(ctx)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	if app.PlatformVersion != "machines" {
		return fmt.Errorf("wrong platform version")
	}

	var region *api.Region
	region, err = prompt.Region(ctx, !app.Organization.PaidPlan, prompt.RegionParams{
		Message: "Select a region. Prefer closer to the primary",
	})
	if err != nil {
		return err
	}

	machineConfig := api.MachineConfig{}

	machineConfig.Env = map[string]string{
		"IS_BARMAN":      "true",
		"PRIMARY_REGION": region.Code,
	}

	// Set VM resources
	vmSizeString := flag.GetString(ctx, "vm-size")
	vmSize, err := resolveVMSize(ctx, vmSizeString)
	if err != nil {
		return err
	}
	machineConfig.Guest = &api.MachineGuest{
		CPUKind:  vmSize.CPUClass,
		CPUs:     int(vmSize.CPUCores),
		MemoryMB: vmSize.MemoryMB,
	}

	// Metrics
	machineConfig.Metrics = &api.MachineMetrics{
		Path: "/metrics",
		Port: 9187,
	}

	machineConfig.Checks = map[string]api.MachineCheck{
		"connection": {
			Port:     api.Pointer(5500),
			Type:     api.Pointer("http"),
			HTTPPath: &CheckPathConnection,
			Interval: &api.Duration{Duration: Duration15s},
			Timeout:  &api.Duration{Duration: Duration10s},
		},
		"role": {
			Port:     api.Pointer(5500),
			Type:     api.Pointer("http"),
			HTTPPath: &CheckPathRole,
			Interval: &api.Duration{Duration: Duration15s},
			Timeout:  &api.Duration{Duration: Duration10s},
		},
		"vm": {
			Port:     api.Pointer(5500),
			Type:     api.Pointer("http"),
			HTTPPath: &CheckPathVm,
			Interval: &api.Duration{Duration: Duration15s},
			Timeout:  &api.Duration{Duration: Duration10s},
		},
	}

	// Metadata
	machineConfig.Metadata = map[string]string{
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyManagedPostgres: "true",
		"managed-by-fly-deploy":                        "true",
		"fly-barman":                                   "true",
	}

	// Restart policy
	machineConfig.Restart.Policy = api.MachineRestartPolicyAlways

	imageRepo := "flyio/postgres-flex"

	imageRef, err := client.GetLatestImageTag(ctx, imageRepo, nil)
	if err != nil {
		return err
	}
	machineConfig.Image = imageRef

	var vol *api.Volume

	volInput := api.CreateVolumeInput{
		AppID:             app.ID,
		Name:              volumeName,
		Region:            region.Code,
		SizeGb:            flag.GetInt(ctx, "volume-size"),
		Encrypted:         true,
		RequireUniqueZone: true,
	}

	if volInput.SizeGb == 0 {
		otherVolumes, err := client.GetVolumes(ctx, app.Name)
		if err != nil {
			return err
		}

		suggestedSize := 1
		for _, volume := range otherVolumes {
			if volume.Name == "pg_data" {
				suggestedSize = volume.SizeGb
			}
		}

		if err = prompt.Int(ctx, &volInput.SizeGb, "Volume size (should be at least the size of the other volumes)", suggestedSize, false); err != nil {
			return err
		}
	}

	fmt.Fprintf(io.Out, "Provisioning volume with %dGB\n", volInput.SizeGb)

	vol, err = client.CreateVolume(ctx, volInput)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}

	machineConfig.Mounts = append(machineConfig.Mounts, api.MachineMount{
		Volume: vol.ID,
		Path:   volumePath,
	})

	launchInput := api.LaunchMachineInput{
		Region: volInput.Region,
		Config: &machineConfig,
	}

	fmt.Fprintf(io.Out, "Provisioning barman machine with image %s\n", machineConfig.Image)

	flapsClient, err := flaps.New(ctx, app)
	machine, err := flapsClient.Launch(ctx, launchInput)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Waiting for machine to start...\n")

	waitTimeout := time.Minute * 5

	err = mach.WaitForStartOrStop(ctx, machine, "start", waitTimeout)
	if err != nil {
		return err
	}
	fmt.Fprintf(io.Out, "Machine %s is %s\n", machine.ID, machine.State)

	return nil
}
