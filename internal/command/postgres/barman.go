package postgres

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/ip"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/sentry"
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
		newCheckBarman(),
		newBarmanListBackup(),
		newBarmanShowBackup(),
		newBarmanBackup(),
		newBarmanSwitchWal(),
		newBarmanRecover(),
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
		api.MachineConfigMetadataKeyFlyctlVersion:      buildinfo.ParsedVersion().String(),
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
		Name:   "barman",
		Region: volInput.Region,
		Config: &machineConfig,
	}

	fmt.Fprintf(io.Out, "Provisioning barman machine with image %s\n", machineConfig.Image)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}
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

func newCheckBarman() *cobra.Command {
	const (
		long  = `Check your barman connection`
		short = long
		usage = "check"
	)

	cmd := command.New(usage, short, long, runBarmanCheck, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func newBarmanListBackup() *cobra.Command {
	const (
		long  = `List your barman backups`
		short = long
		usage = "list-backup"
	)

	cmd := command.New(usage, short, long, runBarmanListBackup, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func newBarmanShowBackup() *cobra.Command {
	const (
		long  = `Show a single barman backup`
		short = long
		usage = "show-backup <backup-id>"
	)

	cmd := command.New(usage, short, long, runBarmanShowBackup, command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func newBarmanBackup() *cobra.Command {
	const (
		long  = `Backup your database on barman`
		short = long
		usage = "backup"
	)

	cmd := command.New(usage, short, long, runBarmanBackup, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func newBarmanSwitchWal() *cobra.Command {
	const (
		long  = `Switch WAL to sync barman`
		short = long
		usage = "switch-wal"
	)

	cmd := command.New(usage, short, long, runBarmanSwitchWal, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func newBarmanRecover() *cobra.Command {
	const (
		long  = `Recover primary database with a barman backup`
		short = long
		usage = "recover <primary machine ID>"
	)

	cmd := command.New(usage, short, long, runBarmanRecover, command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "backup-id",
			Shorthand:   "b",
			Default:     "latest",
			Description: "choose one backup ID. Default: latest",
		},
		flag.String{
			Name:        "target-time",
			Shorthand:   "T",
			Description: "choose a target time for PITR. Example: \"2023-05-16 20:55:05.958774+00:00\". Default: last WAL file on barman",
		},
	)

	return cmd
}

func captureError(err error, app *api.AppCompact) {
	// ignore cancelled errors
	if errors.Is(err, context.Canceled) {
		return
	}

	sentry.CaptureException(err,
		sentry.WithTag("feature", "ssh-console"),
		sentry.WithContexts(map[string]sentry.Context{
			"app": map[string]interface{}{
				"name": app.Name,
			},
			"organization": map[string]interface{}{
				"name": app.Organization.Slug,
			},
		}),
	)
}

func runBarmanCheck(ctx context.Context) error {
	return runConsole(ctx, "barman check pg")
}

func runBarmanListBackup(ctx context.Context) error {
	return runConsole(ctx, "barman list-backup pg")
}

func runBarmanShowBackup(ctx context.Context) error {
	var (
		io = iostreams.FromContext(ctx)
	)
	backupId := flag.FirstArg(ctx)
	fmt.Printf("barman show-backup pg %s", backupId)
	fmt.Fprintf(io.Out, "barman show-backup pg %s", backupId)
	return runConsole(ctx, fmt.Sprintf("barman show-backup pg %s", backupId))
}

func runBarmanBackup(ctx context.Context) error {
	return runConsole(ctx, "barman backup pg")
}

func runBarmanSwitchWal(ctx context.Context) error {
	return runConsole(ctx, "barman switch-wal pg --force --archive")
}

func runBarmanRecover(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	backupId := flag.GetString(ctx, "backup-id")
	targetTime := flag.GetString(ctx, "target-time")
	primaryMachineId := flag.FirstArg(ctx)

	remoteSshCommand := fmt.Sprintf("--remote-ssh-command \"ssh root@%s.vm.%s.internal -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null\" ", primaryMachineId, appName)
	if targetTime != "" {
		remoteSshCommand += fmt.Sprintf("--target-time \"%s\"", targetTime)
	}

	cmd := fmt.Sprintf("barman recover pg --get-wal %s /data/postgresql %s", backupId, remoteSshCommand)

	log.Printf("Barman server will run the following command: %s", cmd)

	return runConsole(ctx, cmd)
}

func runConsole(ctx context.Context, cmd string) error {
	client := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, dialer, err := ssh.BringUpAgent(ctx, client, app, false)
	if err != nil {
		return err
	}

	addr, err := lookupAddress(ctx, agentclient, dialer, app, true)
	if err != nil {
		return err
	}

	params := &ssh.ConnectParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		Username:       "root",
		DisableSpinner: false,
	}
	sshc, err := ssh.Connect(params, addr)
	if err != nil {
		captureError(err, app)
		return err
	}

	if err := ssh.Console(ctx, sshc, cmd, false); err != nil {
		captureError(err, app)
		return err
	}

	return nil
}

func lookupAddress(ctx context.Context, cli *agent.Client, dialer agent.Dialer, app *api.AppCompact, console bool) (addr string, err error) {
	addr, err = addrForMachines(ctx, app, console)

	if err != nil {
		return
	}

	// wait for the addr to be resolved in dns unless it's an ip address
	if !ip.IsV6(addr) {
		if err := cli.WaitForDNS(ctx, dialer, app.Organization.Slug, addr); err != nil {
			captureError(err, app)
			return "", errors.Wrapf(err, "host unavailable at %s", addr)
		}
	}

	return
}

func addrForMachines(ctx context.Context, app *api.AppCompact, console bool) (addr string, err error) {
	// out := iostreams.FromContext(ctx).Out
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return "", err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return "", err
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return m.State == "started"
	})

	if len(machines) < 1 {
		return "", fmt.Errorf("app %s has no started VMs", app.Name)
	}

	if err != nil {
		return "", err
	}

	var selectedMachine *api.Machine

	for _, machine := range machines {
		if machine.Config.Env["IS_BARMAN"] != "" {
			selectedMachine = machine
		}
	}

	if selectedMachine == nil {
		return "", fmt.Errorf("no barman machine found")
	}
	// No VM was selected or passed as an argument, so just pick the first one for now
	// Later, we might want to use 'nearest.of' but also resolve the machine IP to be able to start it
	return selectedMachine.PrivateIP, nil
}
