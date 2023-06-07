package migrate_to_v2

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/Khan/genqlient/graphql"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func New() *cobra.Command {
	return newMigrateToV2()
}

func newMigrateToV2() *cobra.Command {
	const (
		usage = `migrate-to-v2`
		short = `Migrate a v1 app to v2`
		long  = `Migrates an Apps V1 (Nomad) app to the Apps V2 (machines) platform`
	)
	cmd := command.New(
		usage, short, long, runMigrateToV2,
		command.RequireSession,
		command.LoadAppNameIfPresent,
		command.LoadAppConfigIfPresent,
		func(ctx context.Context) (context.Context, error) {
			if appName := appconfig.NameFromContext(ctx); appName != "" {
				return ctx, nil
			}

			if cfg := appconfig.ConfigFromContext(ctx); cfg != nil {
				if cfg.AppName == "" {
					return nil, fmt.Errorf("your fly.toml is missing an app name, please ensure the 'app' field is set")
				}
			} else {
				return nil, fmt.Errorf("no config found, please ensure there is a fly.toml in the current directory or pass one with '-c <path>'")
			}
			return ctx, nil
		},
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.Yes(),
		flag.AppConfig(),
		flag.String{
			Name:        "primary-region",
			Description: "Specify primary region if one is not set in fly.toml",
		},
	)

	cmd.AddCommand(newTroubleshoot())

	return cmd
}

func runMigrateToV2(ctx context.Context) (err error) {
	var (
		appName   = appconfig.NameFromContext(ctx)
		apiClient = client.FromContext(ctx).API()
	)

	// pre-fetch platform regions for later use
	prompt.PlatformRegions(ctx)

	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	// BuildContext() creates a wg tunnel that is only used for PG migrations
	if appCompact.IsPostgresApp() {
		ctx, err = apps.BuildContext(ctx, appCompact)
		if err != nil {
			return err
		}
	} else {
		flapsClient, err := flaps.New(ctx, appCompact)
		if err != nil {
			return err
		}
		ctx = flaps.NewContext(ctx, flapsClient)
	}

	// This is written awkwardly so that NewV2PlatformMigrator failures are tracked,
	// but declined migrations are not.
	sendMetric := true
	defer func() {
		if sendMetric {
			metrics.Started(ctx, "migrate_to_v2")
			metrics.Status(ctx, "migrate_to_v2", err == nil)
		}
	}()

	migrator, err := NewV2PlatformMigrator(ctx, appName)
	if err != nil {
		return err
	}
	if !flag.GetYes(ctx) {
		confirm, err := migrator.ConfirmChanges(ctx)
		if err != nil {
			return err
		}
		if !confirm {
			sendMetric = false
			return nil
		}
	}
	err = migrator.Migrate(ctx)
	if err != nil {
		sentry.CaptureException(err,
			sentry.WithTag("feature", "migrate-to-v2"),
			sentry.WithContexts(map[string]sentry.Context{
				"app": {
					"name": appName,
				},
				"organization": {
					"name": appCompact.Organization.Slug,
				},
			}),
		)
		return err
	}
	return nil
}

type V2PlatformMigrator interface {
	ConfirmChanges(ctx context.Context) (bool, error)
	Migrate(ctx context.Context) error
}

type NewVolume struct {
	vol             *api.Volume
	previousAllocId string
	mountPoint      string
}

// FIXME: a lot of stuff is shared with MachineDeployment... might be useful to consolidate the shared stuff into another library/package/something
type v2PlatformMigrator struct {
	apiClient               *api.Client
	flapsClient             *flaps.Client
	gqlClient               graphql.Client
	io                      *iostreams.IOStreams
	colorize                *iostreams.ColorScheme
	leaseTimeout            time.Duration
	leaseDelayBetween       time.Duration
	appCompact              *api.AppCompact
	appFull                 *api.App
	appConfig               *appconfig.Config
	configPath              string
	autoscaleConfig         *api.AutoscalingConfig
	formattedProcessConfigs string
	img                     string
	appLock                 string
	releaseId               string
	releaseVersion          int
	oldAllocs               []*api.AllocationStatus
	oldAttachedVolumes      []api.Volume
	machineGuests           map[string]*api.MachineGuest
	oldVmCounts             map[string]int
	newMachinesInput        []*api.LaunchMachineInput
	newMachines             machine.MachineSet
	recovery                recoveryState
	usesForkedVolumes       bool
	createdVolumes          []*NewVolume
	replacedVolumes         map[string]int
	isPostgres              bool
	pgConsulUrl             string
	targetImg               string
}

type recoveryState struct {
	machinesCreated []*api.Machine
	appLocked       bool
	scaledToZero    bool
	platformVersion string
}

func NewV2PlatformMigrator(ctx context.Context, appName string) (V2PlatformMigrator, error) {
	var (
		apiClient = client.FromContext(ctx).API()
		io        = iostreams.FromContext(ctx)
		colorize  = io.ColorScheme()
	)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, err
	}
	flapsClient, err := flaps.New(ctx, appCompact)
	if err != nil {
		return nil, err
	}
	appFull, err := apiClient.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	if appFull.PlatformVersion == "machines" {
		return nil, fmt.Errorf("the app '%s' is already on the apps v2 platform", appName)
	}
	appConfig, err := determineAppConfigForMachines(ctx)
	if err != nil {
		return nil, err
	}
	formattedProcessConfigs := appConfig.FormatProcessNames()
	if err != nil {
		return nil, err
	}
	autoscaleConfig, err := apiClient.AppAutoscalingConfig(ctx, appName)
	if err != nil {
		return nil, err
	}
	imageInfo, err := apiClient.GetImageInfo(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get image info: %w", err)
	}
	var img string
	switch {
	case imageInfo.ImageDetails.Tag != "":
		img = fmt.Sprintf("%s/%s:%s", imageInfo.ImageDetails.Registry, imageInfo.ImageDetails.Repository, imageInfo.ImageDetails.Tag)
	case imageInfo.ImageDetails.Digest != "":
		img = fmt.Sprintf("%s/%s@%s", imageInfo.ImageDetails.Registry, imageInfo.ImageDetails.Repository, imageInfo.ImageDetails.Digest)
	default:
		return nil, fmt.Errorf("failed to get image info: no tag or digest found")
	}
	allocs, err := apiClient.GetAllocations(ctx, appName, false)
	if err != nil {
		return nil, err
	}
	allocs = lo.Filter(allocs, func(alloc *api.AllocationStatus, _ int) bool {
		return alloc.Status == "running" && alloc.LatestVersion
	})

	attachedVolumes := lo.Filter(appFull.Volumes.Nodes, func(v api.Volume, _ int) bool {
		if v.AttachedAllocation != nil {
			for _, a := range allocs {
				if a.ID == v.AttachedAllocation.ID {
					return true
				}
			}
		}
		return false
	})

	vmSize, _, groups, err := apiClient.AppVMResources(ctx, appName)
	if err != nil {
		return nil, err
	}
	machineGuests, err := determineVmSpecs(vmSize, groups)
	if err != nil {
		return nil, err
	}
	leaseTimeout := 13 * time.Second
	leaseDelayBetween := (leaseTimeout - 1*time.Second) / 3
	migrator := &v2PlatformMigrator{
		apiClient:               apiClient,
		flapsClient:             flapsClient,
		gqlClient:               apiClient.GenqClient,
		io:                      io,
		colorize:                colorize,
		leaseTimeout:            leaseTimeout,
		leaseDelayBetween:       leaseDelayBetween,
		appCompact:              appCompact,
		appFull:                 appFull,
		appConfig:               appConfig,
		autoscaleConfig:         autoscaleConfig,
		formattedProcessConfigs: formattedProcessConfigs,
		img:                     img,
		oldAllocs:               allocs,
		oldAttachedVolumes:      attachedVolumes,
		machineGuests:           machineGuests,
		isPostgres:              appCompact.IsPostgresApp(),
		replacedVolumes:         map[string]int{},
		recovery: recoveryState{
			platformVersion: appFull.PlatformVersion,
		},
	}
	if migrator.isPostgres {
		consul, err := apiClient.EnablePostgresConsul(ctx, appCompact.Name)
		if err != nil {
			return nil, err
		}
		migrator.pgConsulUrl = consul.ConsulURL
	}
	err = migrator.validate(ctx)
	if err != nil {
		return nil, err
	}
	err = migrator.determinePrimaryRegion(ctx)
	if err != nil {
		return nil, err
	}
	err = migrator.prepMachinesToCreate(ctx)
	if err != nil {
		return nil, err
	}
	err = migrator.determineConfigPath(ctx)
	if err != nil {
		return nil, err
	}
	migrator.resolveProcessGroups(ctx)
	return migrator, nil
}

func (m *v2PlatformMigrator) rollback(ctx context.Context, tb *render.TextBlock) error {
	defer func() {
		if m.recovery.appLocked {
			tb.Detail("Unlocking application")
			err := m.unlockApp(ctx)
			if err != nil {
				fmt.Fprintf(m.io.ErrOut, "failed to unlock app: %v\n", err)
			}
		}
		// HACK: machines apps use the suspended state to describe an app with no machines.
		//       this means something different in nomad-land, so we resume just in case this got set.
		_, err := m.apiClient.ResumeApp(ctx, m.appCompact.Name)
		if err != nil {
			if !strings.Contains(err.Error(), "not suspended") {
				fmt.Fprintf(m.io.ErrOut, "failed to unsuspend app: %v\n", err)
			}
		}
	}()

	if len(m.recovery.machinesCreated) > 0 {
		tb.Detailf("Removing machines")
		for _, mach := range m.recovery.machinesCreated {

			input := api.RemoveMachineInput{
				ID:   mach.ID,
				Kill: true,
			}
			err := m.flapsClient.Destroy(ctx, input, mach.LeaseNonce)
			if err != nil {
				return err
			}
		}
	}
	if len(m.createdVolumes) > 0 {
		tb.Detailf("Removing migration-created volumes")
		for _, vol := range m.createdVolumes {
			_, err := m.apiClient.DeleteVolume(ctx, vol.vol.ID, m.appLock)
			if err != nil {
				return err
			}
		}
	}
	if m.isPostgres {
		tb.Detailf("Disabling readonly")
		err := m.setNomadPgReadonly(ctx, false)
		if err != nil {
			return err
		}
	}
	if m.recovery.platformVersion != "nomad" {

		tb.Detailf("Setting platform version to 'nomad'")
		err := m.updateAppPlatformVersion(ctx, "nomad")
		if err != nil {
			return err
		}
	}

	if m.recovery.scaledToZero && len(m.oldAllocs) > 0 {
		// Restore nomad allocs
		tb.Detail("Restoring nomad allocs to their previous state")

		input := gql.SetVMCountInput{
			AppId: m.appConfig.AppName,
			GroupCounts: lo.MapToSlice(m.oldVmCounts, func(name string, count int) gql.VMCountInput {
				return gql.VMCountInput{Group: name, Count: count}
			}),
			LockId: lo.Ternary(m.recovery.appLocked, m.appLock, ""),
		}

		_, err := gql.SetNomadVMCount(ctx, m.gqlClient, input)
		if err != nil {
			return err
		}
	}
	tb.Detail("Successfully recovered")
	return nil
}

func (m *v2PlatformMigrator) Migrate(ctx context.Context) (err error) {
	ctx = flaps.NewContext(ctx, m.flapsClient)

	tb := render.NewTextBlock(ctx, fmt.Sprintf("Migrating %s to the V2 platform", m.appCompact.Name))

	m.recovery.platformVersion = m.appFull.PlatformVersion

	abortedErr := errors.New("migration aborted by user")
	defer func() {
		if err != nil {

			header := ""
			if err == abortedErr {
				header = "(!) Received abort signal, restoring application to stable state..."
			} else {
				header = "(!) An error has occurred. Attempting to rollback changes..."
			}
			fmt.Fprintf(m.io.ErrOut, "failed while migrating: %v\n", err)

			enterTroubleshooting := false
			if !flag.GetYes(ctx) && err != abortedErr && m.io.IsInteractive() {
				askErr := survey.AskOne(&survey.Confirm{
					Message: "Would you like to enter interactive troubleshooting mode? If not, the migration will be rolled back.",
					Default: true,
				}, &enterTroubleshooting)
				if askErr != nil {
					enterTroubleshooting = false
				}
			}

			if enterTroubleshooting {

				migrateErr := func() error {
					t, err := newTroubleshooter(ctx, m.appCompact.Name)
					if err != nil {
						return err
					}

					return t.run(ctx)
				}()
				if migrateErr != nil {
					fmt.Fprintf(m.io.ErrOut, "failed while troubleshooting: %v\n", err)
				} else {
					err = nil // Printing an error message below a successful troubleshooting run is confusing
				}

			} else {
				recoveryBlock := render.NewTextBlock(ctx, header)
				if recoveryErr := m.rollback(ctx, recoveryBlock); recoveryErr != nil {
					fmt.Fprintf(m.io.ErrOut, "failed while rolling back application: %v\n", recoveryErr)
				}
			}
		}
	}()

	aborted := atomic.Bool{}
	// Hook into Ctrl+C so that aborting the migration
	// leaves the app in a stable, unlocked, non-detached state
	{
		signalCh := make(chan os.Signal, 1)
		abortSignals := []os.Signal{os.Interrupt}
		if runtime.GOOS != "windows" {
			abortSignals = append(abortSignals, syscall.SIGTERM)
		}
		// Prevent ctx from being cancelled, we need it to do recovery operations
		signal.Reset(abortSignals...)
		signal.Notify(signalCh, abortSignals...)
		go func() {
			<-signalCh
			// most terminals print ^C, this makes things easier to read.
			fmt.Fprintf(m.io.ErrOut, "\n")
			aborted.Store(true)
		}()
	}

	if m.isPostgres {
		tb.Detail("Upgrading postgres image")
		err := m.updateNomadPostgresImage(ctx)
		if err != nil {
			return err
		}

		tb.Detail("Setting postgres primary to readonly")
		err = m.setNomadPgReadonly(ctx, true)
		if err != nil {
			return err
		}

		tb.Detail("Creating new postgres volumes")
		err = m.migratePgVolumes(ctx)
		if err != nil {
			return err
		}
	}

	tb.Detail("Locking app to prevent changes during the migration")
	err = m.lockApp(ctx)
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}

	if !m.isPostgres && m.usesForkedVolumes {
		tb.Detail("Making snapshots of volumes for the new machines")
		err = m.migrateAppVolumes(ctx)
		if err != nil {
			return err
		}
		if aborted.Load() {
			return abortedErr
		}
	}

	if m.requiresDowntime() {
		tb.Detail("Scaling down to zero VMs. This will cause temporary downtime until new VMs come up.")

		err = m.scaleNomadToZero(ctx)
		if err != nil {
			return err
		}
	}
	if aborted.Load() {
		return abortedErr
	}

	tb.Detail("Enabling machine creation on app")

	err = m.updateAppPlatformVersion(ctx, "detached")
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}

	tb.Detail("Creating an app release to register this migration")

	err = m.createRelease(ctx)
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}

	tb.Detail("Starting machines")

	err = m.createMachines(ctx)
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}

	err = m.newMachines.AcquireLeases(ctx, m.leaseTimeout)
	defer func() {
		err := m.newMachines.ReleaseLeases(ctx)
		if err != nil {
			terminal.Warnf("error releasing leases on machines: %v\n", err)
		}
	}()
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}
	m.newMachines.StartBackgroundLeaseRefresh(ctx, m.leaseTimeout, m.leaseDelayBetween)

	err = m.unlockApp(ctx)
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}

	m.newMachines.ReleaseLeases(ctx)
	err = m.deployApp(ctx)
	if err != nil {
		return err
	}
	if aborted.Load() {
		return abortedErr
	}

	if m.isPostgres {
		inRegionDbs := m.inRegionMachines()

		tb.Detail("Waiting for new potential primaries to become healthy")
		err = m.waitForHealthyPgs(ctx)
		if err != nil {
			return err
		}

		var dbUIDs []string
		tb.Detail("Collecting info about the new pg cluster")
		dbUIDs, err = m.getPgDBUids(ctx, inRegionDbs)
		if err != nil {
			return err
		}

		tb.Detailf("We are checking to see if the following DB UIDs are synced %+v", dbUIDs)

		err = m.waitForPGSync(ctx, dbUIDs)
		if err != nil {
			return err
		}
	}

	if !m.requiresDowntime() {
		tb.Detail("Scaling nomad VMs down to zero now that machines are running.")

		err = m.scaleNomadToZero(ctx)
		if err != nil {
			return err
		}
		if aborted.Load() {
			return abortedErr
		}
	}

	if m.isPostgres {
		tb.Detail("Waiting for election so we can disable readonly")
		err = m.waitForElection(ctx)
		if err != nil {
			return err
		}
	}

	tb.Detail("Updating the app platform platform type from V1 to V2")

	err = m.updateAppPlatformVersion(ctx, "machines")
	if err != nil {
		return err
	}

	tb.Detail("Saving new configuration")

	var configSaveErr error

	if !m.isPostgres {
		configSaveErr = m.appConfig.WriteToDisk(ctx, m.configPath)
	}

	tb.Done("Done")
	m.printReplacedVolumes()

	if configSaveErr != nil {
		fmt.Fprintf(m.io.ErrOut, "Failed to save application config to disk, but migration was successful.\n")
		fmt.Fprintf(m.io.ErrOut, "Please run `fly config save` before further interacting with your app via flyctl.\n")
	}

	return nil
}

func (m *v2PlatformMigrator) inRegionMachines() []*api.Machine {
	return lo.FilterMap(m.newMachines.GetMachines(), func(mach machine.LeasableMachine, _ int) (*api.Machine, bool) {
		if mach.Machine().Region == m.appConfig.PrimaryRegion {
			return mach.Machine(), true
		}
		return nil, false
	})
}

func (m *v2PlatformMigrator) validate(ctx context.Context) error {
	if err := m.validateKnownUnmigratableApps(ctx); err != nil {
		return err
	}

	var err error
	err, extraInfo := m.appConfig.ValidateForMachinesPlatform(ctx)
	if err != nil {
		fmt.Println(extraInfo)
		return fmt.Errorf("failed to validate config for Apps V2 platform: %w", err)
	}

	err = m.validateScaling(ctx)
	if err != nil {
		return err
	}
	err = m.validateVolumes(ctx)
	if err != nil {
		return err
	}
	err = m.validateProcessGroupsOnAllocs(ctx)
	if err != nil {
		return err
	}
	if m.isPostgres {
		err = m.validatePgSettings(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *v2PlatformMigrator) validateKnownUnmigratableApps(ctx context.Context) error {
	// This is as last meassure for very special cases
	knownUnmigratableApps := []string{
		"globalconfig-dht", // https://flyio.discourse.team/t/x/2860/6
	}
	if slices.Contains(knownUnmigratableApps, m.appCompact.ID) {
		return fmt.Errorf("Your app uses features incompatible with the V2 platform. Please contact support to discuss how to successfully migrate")
	}
	return nil
}

func (m *v2PlatformMigrator) validateScaling(ctx context.Context) error {
	// FIXME: for now we fail if there is autoscaling.. remove this once we create any extra machines based on autoscaling config
	if m.autoscaleConfig.Enabled {
		return fmt.Errorf("cannot migrate app %s with autoscaling config, yet; watch https://community.fly.io for announcements about autoscale support with migrations", m.appCompact.Name)
	}
	return nil
}

func (m *v2PlatformMigrator) validateProcessGroupsOnAllocs(ctx context.Context) error {
	validGroupNames := m.appConfig.ProcessNames()
	for _, a := range m.oldAllocs {
		if !slices.Contains(validGroupNames, a.TaskName) {
			return fmt.Errorf("alloc %s has process group '%s' that is not present in app configuration fly.toml; known process groups are: %s", a.IDShort, a.TaskName, m.formattedProcessConfigs)
		}
	}
	return nil
}

func (m *v2PlatformMigrator) createRelease(ctx context.Context) error {
	_ = `# @genqlient
	mutation MigrateMachinesCreateRelease($input:CreateReleaseInput!) {
		createRelease(input:$input) {
			release {
				id
				version
			}
		}
	}
	`
	input := gql.CreateReleaseInput{
		AppId:           m.appConfig.AppName,
		PlatformVersion: "machines",
		Strategy:        gql.DeploymentStrategy(strings.ToUpper("simple")),
		Definition:      m.appConfig,
		Image:           m.img,
	}
	resp, err := gql.MigrateMachinesCreateRelease(ctx, m.gqlClient, input)
	if err != nil {
		return err
	}
	m.releaseId = resp.CreateRelease.Release.Id
	m.releaseVersion = resp.CreateRelease.Release.Version
	return nil
}

func (m *v2PlatformMigrator) resolveProcessGroups(ctx context.Context) {
	m.oldVmCounts = map[string]int{}
	for _, alloc := range m.oldAllocs {
		m.oldVmCounts[alloc.TaskName] += 1
	}
}

func (m *v2PlatformMigrator) updateAppPlatformVersion(ctx context.Context, platform string) error {
	_ = `# @genqlient
	mutation SetPlatformVersion($input:SetPlatformVersionInput!) {
		setPlatformVersion(input:$input) {
			app { id }
		}
	}
	`
	input := gql.SetPlatformVersionInput{
		AppId:           m.appConfig.AppName,
		PlatformVersion: platform,
		LockId:          m.appLock,
	}
	_, err := gql.SetPlatformVersion(ctx, m.gqlClient, input)
	if err != nil {
		return err
	}
	m.recovery.platformVersion = platform
	return nil
}

func (m *v2PlatformMigrator) deployApp(ctx context.Context) error {
	input := deploy.MachineDeploymentArgs{
		AppCompact:  m.appCompact,
		RestartOnly: true,
	}
	if m.isPostgres {
		if len(m.appConfig.Mounts) > 0 {
			m.appConfig.Mounts[0].Source = "pg_data_machines"
		}
	}
	// Feed appConfig into the context so MachineDeployment can reuse it
	ctx = appconfig.WithConfig(ctx, m.appConfig)
	md, err := deploy.NewMachineDeployment(ctx, input)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(err, "migrate-to-v2", m.appCompact)
		return err
	}
	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(err, "migrate-to-v2", m.appCompact)
	}
	return nil
}

func (m *v2PlatformMigrator) requiresDowntime() bool {
	return m.usesForkedVolumes
}

func (m *v2PlatformMigrator) determinePrimaryRegion(ctx context.Context) error {
	if fromFlag := flag.GetString(ctx, "primary-region"); fromFlag != "" {
		m.appConfig.PrimaryRegion = fromFlag
		return nil
	}

	if val, ok := m.appConfig.Env["PRIMARY_REGION"]; ok {
		m.appConfig.PrimaryRegion = val
		return nil
	}

	existingRegions := map[string]struct{}{}
	for _, alloc := range m.oldAllocs {
		existingRegions[alloc.Region] = struct{}{}
	}

	if len(existingRegions) == 1 {
		m.appConfig.PrimaryRegion = maps.Keys(existingRegions)[0]
		return nil
	}

	// TODO: If this ends up used by postgres migrations, it might be nice to have
	//       the prompt here reflect the special role `primary_region` plays for postgres apps

	region, err := prompt.Region(ctx, !m.appFull.Organization.PaidPlan, prompt.RegionParams{
		Message: "Choose the primary region for this app:",
	})
	if err != nil {
		return err
	}
	if region == nil {
		return errors.New("no region provided")
	}
	m.appConfig.PrimaryRegion = region.Code
	return nil
}

func (m *v2PlatformMigrator) determineConfigPath(ctx context.Context) error {
	path := state.WorkingDirectory(ctx)
	if flag.IsSpecified(ctx, "config") {
		path = flag.GetString(ctx, "config")
	}
	configPath, err := appconfig.ResolveConfigFileFromPath(path)
	if err != nil {
		return err
	}

	m.configPath = configPath
	return nil
}

func (m *v2PlatformMigrator) ConfirmChanges(ctx context.Context) (bool, error) {
	numAllocs := len(m.oldAllocs)

	fmt.Fprintf(m.io.Out, "This migration process will do the following, in order:\n")
	if m.isPostgres {
		fmt.Fprintf(m.io.Out, " * Update your postgres app to the latest supported image version\n")
		fmt.Fprintf(m.io.Out, " * Create volumes for new PG replicas to use\n")
	}
	fmt.Fprintf(m.io.Out, " * Lock your application, preventing changes during the migration\n")

	printAllocs := func(msgSuffix string) {
		fmt.Fprintf(m.io.Out, " * Remove legacy VMs %s\n", msgSuffix)
		if numAllocs > 0 {
			s := "s"
			if numAllocs == 1 {
				s = ""
			}
			fmt.Fprintf(m.io.Out, "   * Remove %d alloc%s\n", numAllocs, s)
		}
	}

	if m.requiresDowntime() {
		printAllocs("")
		colorize := m.io.ColorScheme()
		fmt.Fprintf(m.io.Out, "   * %s: Because your app uses volumes, there will be a short downtime during migration while your machines start up.\n", colorize.Bold(colorize.Yellow("NOTE")))
	}
	if m.usesForkedVolumes {
		fmt.Fprintf(m.io.Out, " * Create clones of each volume in use, for the new machines\n")
		fmt.Fprintf(m.io.Out, "   * These cloned volumes will have the suffix '%s' appended to their names\n", forkedVolSuffix)
		fmt.Fprintf(m.io.Out, "   * Please note that your old volumes will not be removed.\n")
		fmt.Fprintf(m.io.Out, "     (you can do this manually, after making sure the migration was a success)\n")
	}

	fmt.Fprintf(m.io.Out, " * Create machines, copying the configuration of each existing VM\n")
	for name, count := range m.oldVmCounts {
		s := "s"
		if count == 1 {
			s = ""
		}
		fmt.Fprintf(m.io.Out, "   * Create %d \"%s\" machine%s\n", count, name, s)
	}

	if m.isPostgres {
		fmt.Fprintf(m.io.Out, " * Wait for at least one new in-region PG replica to become synced\n")
	}

	if !m.requiresDowntime() {
		printAllocs("after health checks pass for the new machines")
	}

	fmt.Fprintf(m.io.Out, " * Set the application platform version to \"machines\"\n")
	fmt.Fprintf(m.io.Out, " * Unlock your application\n")

	if !m.isPostgres {
		if exists, _ := appconfig.ConfigFileExistsAtPath(m.configPath); exists {
			fmt.Fprintf(m.io.Out, " * Overwrite the config file at '%s'\n", m.configPath)
		} else {
			fmt.Fprintf(m.io.Out, " * Save the app's config file to '%s'\n", m.configPath)
		}
	}

	confirm := false
	prompt := &survey.Confirm{
		Message: "Would you like to continue?",
	}
	err := survey.AskOne(prompt, &confirm)

	return confirm, err
}

func determineAppConfigForMachines(ctx context.Context) (*appconfig.Config, error) {
	appNameFromContext := appconfig.NameFromContext(ctx)

	// We're pulling the remote config because we don't want to inadvertently trigger a new deployment -
	// people will expect this to migrate what's _currently_ live.
	// That said, we need to reference the local config to get the build config, because it's
	// sanitized out before being sent to the API.
	localAppConfig := appconfig.ConfigFromContext(ctx)
	cfg, err := appconfig.FromRemoteApp(ctx, appNameFromContext)
	if err != nil {
		return nil, err
	}
	if appNameFromContext != "" {
		cfg.AppName = appNameFromContext
	}
	if localAppConfig != nil {
		cfg.Build = localAppConfig.Build
	}
	return cfg, nil
}

func determineVmSpecs(defaultSize api.VMSize, groups []api.ProcessGroup) (map[string]*api.MachineGuest, error) {
	mapSize := func(size api.VMSize) (*api.MachineGuest, error) {
		preset := size.Name
		preset = strings.Replace(preset, "micro", "shared-cpu", 1)
		preset = strings.Replace(preset, "dedicated-cpu", "performance", 1)
		switch preset {
		case "cpu1mem1":
			preset = "performance-1x"
		case "cpu2mem2":
			preset = "performance-2x"
		case "cpu4mem4":
			preset = "performance-4x"
		case "cpu8mem8":
			preset = "performance-8x"
		}

		guest := &api.MachineGuest{}
		if err := guest.SetSize(preset); err != nil {
			return nil, fmt.Errorf("nomad VM definition incompatible with machines API: %w", err)
		}

		// Can't set less memory than the preset
		if size.MemoryMB > guest.MemoryMB {
			guest.MemoryMB = size.MemoryMB
		}

		// minimum memory for a machine is 256MB, micro-1x on V1 allowed 128MB
		if guest.MemoryMB < 256 {
			guest.MemoryMB = 256
		}

		return guest, nil
	}
	sizes := make(map[string]*api.MachineGuest)
	if len(groups) == 0 {
		groups = []api.ProcessGroup{{Name: "app", VMSize: &defaultSize}}
	}
	for _, group := range groups {
		size, err := mapSize(*group.VMSize)
		if err != nil {
			return nil, err
		}
		sizes[group.Name] = size
	}
	return sizes, nil
}
