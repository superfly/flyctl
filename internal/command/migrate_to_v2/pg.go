package migrate_to_v2

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/jpillora/backoff"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/watch"
)

func (m *v2PlatformMigrator) updateNomadPostgresImage(ctx context.Context) error {
	app, err := m.apiClient.GetImageInfo(ctx, m.appCompact.Name)
	if err != nil {
		return fmt.Errorf("failed to get image info: %w", err)
	}

	if !app.ImageVersionTrackingEnabled {
		return errors.New("image is not eligible for automated image updates")
	}

	if !app.ImageUpgradeAvailable {
		return nil
	}

	lI := app.LatestImageDetails

	input := api.DeployImageInput{
		AppID:    m.appCompact.Name,
		Image:    lI.FullImageRef(),
		Strategy: api.StringPointer("ROLLING"),
	}

	m.targetImg = lI.FullImageRef()

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ReplaceAll(strings.ToUpper(val), "-", "_"))
	}

	release, releaseCommand, err := m.apiClient.DeployImage(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(m.io.Out, "Release v%d created\n", release.Version)
	if releaseCommand != nil {
		fmt.Fprintln(m.io.Out, "Release command detected: this new release will not be available until the command succeeds.")
	}

	fmt.Fprintln(m.io.Out)

	if releaseCommand != nil {
		// TODO: don't use text block here
		tb := render.NewTextBlock(ctx, fmt.Sprintf("Release command detected: %s\n", releaseCommand.Command))
		tb.Done("This release will not be available until the release command succeeds.")

		if err := watch.ReleaseCommand(ctx, m.appCompact.Name, releaseCommand.ID); err != nil {
			return err
		}

		release, err = m.apiClient.GetAppReleaseNomad(ctx, m.appCompact.Name, release.ID)
		if err != nil {
			return err
		}
	}
	return watch.Deployment(ctx, m.appCompact.Name, release.EvaluationID)
}

func (m *v2PlatformMigrator) migratePgVolumes(ctx context.Context) error {
	app := m.appFull
	regionsToVols := map[string][]api.Volume{}
	// Find all volumes
	for _, vol := range app.Volumes.Nodes {
		if strings.Contains(vol.Name, "machines") || vol.AttachedAllocation == nil {
			continue
		}
		regionsToVols[vol.Region] = append(regionsToVols[vol.Region], vol)
	}

	var newVols []*NewVolume
	for region, vols := range regionsToVols {
		fmt.Fprintf(m.io.Out, "Creatings %d new volume(s) in '%s'\n", len(vols), region)
		for _, vol := range vols {
			input := api.CreateVolumeRequest{
				Name:         fmt.Sprintf("%s_machines", vol.Name),
				Region:       region,
				SizeGb:       &vol.SizeGb,
				Encrypted:    api.Pointer(vol.Encrypted),
				MachinesOnly: api.Pointer(true),
			}
			newVol, err := m.flapsClient.CreateVolume(ctx, input)
			if err != nil {
				return err
			}
			newVols = append(newVols, &NewVolume{
				vol:             newVol,
				previousAllocId: *vol.AttachedAllocation,
				mountPoint:      "/data",
			})
		}
	}
	m.createdVolumes = newVols
	return nil
}

func (m *v2PlatformMigrator) waitForElection(ctx context.Context) error {
	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = m.io.ErrOut
	s.Prefix = "Waiting for leader to be elected so we can disable readonly"
	s.Start()

	defer s.Stop()

	timeout := time.After(20 * time.Minute)
	ticker := time.Tick(10 * time.Second)
	for {
		select {
		case <-ticker:
			err := m.disablePgReadonly(ctx)
			if err == nil {
				return nil
			}
		case <-timeout:
			return errors.New("pgs never got healthy, timing out")
		}
	}
}
func leaderIpFromInstances(ctx context.Context, addrs []string) (string, error) {
	dialer := agent.DialerFromContext(ctx)
	for _, addr := range addrs {
		pgclient := flypg.NewFromInstance(addr, dialer)
		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return "", fmt.Errorf("can't get role for %s: %w", addr, err)
		}

		if role == "leader" || role == "primary" {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no instances found with leader role")
}

func (m *v2PlatformMigrator) setNomadPgReadonly(ctx context.Context, enable bool) error {
	dialer := agent.DialerFromContext(ctx)
	agentclient, err := agent.Establish(ctx, m.apiClient)
	if err != nil {
		return err
	}

	pgInstances, err := agentclient.Instances(ctx, m.appFull.Organization.Slug, m.appFull.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", m.appCompact.Name, err)
	}

	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", m.appCompact.Name)
	}

	leaderIP, err := leaderIpFromInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	if enable {
		err = pgclient.LegacyEnableReadonly(ctx)
	} else {
		err = pgclient.LegacyDisableReadonly(ctx)
	}
	if err != nil {
		return err
	}

	for _, instance := range pgInstances.Addresses {
		if instance == leaderIP {
			continue
		}
		pgclient = flypg.NewFromInstance(instance, dialer)
		err = pgclient.LegacyBounceHaproxy(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *v2PlatformMigrator) disablePgReadonly(ctx context.Context) error {
	dialer := agent.DialerFromContext(ctx)

	var addrs []string
	for _, machine := range m.newMachines.GetMachines() {
		addrs = append(addrs, machine.Machine().PrivateIP)
	}

	leaderIP, err := leaderIpFromInstances(ctx, addrs)
	if err != nil {
		return err
	}

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	err = pgclient.LegacyDisableReadonly(ctx)
	if err != nil {
		return err
	}

	err = m.bounceHaproxy(ctx, leaderIP)
	if err != nil {
		return err
	}

	return nil
}

func (m *v2PlatformMigrator) validatePgSettings(ctx context.Context) error {
	dialer := agent.DialerFromContext(ctx)
	agentclient, err := agent.Establish(ctx, m.apiClient)
	if err != nil {
		return err
	}

	pgInstances, err := agentclient.Instances(ctx, m.appFull.Organization.Slug, m.appFull.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", m.appCompact.Name, err)
	}

	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", m.appCompact.Name)
	}

	leaderIP, err := leaderIpFromInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	settings, err := pgclient.ViewSettings(ctx, []string{"max_wal_senders", "max_replication_slots"}, "stolon")
	if err != nil {
		return err
	}

	for _, setting := range settings.Settings {
		if setting.Name == "max_wal_senders" || setting.Name == "max_replication_slots" {
			atoi, err := strconv.Atoi(setting.Setting)
			if err != nil {
				return err
			}
			if atoi < len(m.oldAllocs)*2+1 {
				return fmt.Errorf("max_wal_senders and max_replication_slots need to be set to at least %d", len(m.oldAllocs)*2+1)
			}
		}
	}
	return nil
}

func (m *v2PlatformMigrator) bounceHaproxy(ctx context.Context, leader string) error {
	dialer := agent.DialerFromContext(ctx)
	for _, machine := range m.newMachines.GetMachines() {
		if machine.Machine().PrivateIP == leader {
			continue
		}
		pgclient := flypg.NewFromInstance(machine.Machine().PrivateIP, dialer)
		err := pgclient.LegacyBounceHaproxy(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *v2PlatformMigrator) getPgDBUids(ctx context.Context, dbs []*api.Machine) ([]string, error) {
	var uids []string
	dialer := agent.DialerFromContext(ctx)
	for _, machine := range dbs {
		pgclient := flypg.NewFromInstance(machine.PrivateIP, dialer)
		uid, err := pgclient.LegacyStolonDBUid(ctx)
		if err != nil {
			return nil, err
		}
		uids = append(uids, *uid)
	}
	return uids, nil
}

func (m *v2PlatformMigrator) checkPgSync(ctx context.Context, dbuids []string) (*bool, error) {
	dialer := agent.DialerFromContext(ctx)
	agentclient, err := agent.Establish(ctx, m.apiClient)
	if err != nil {
		return nil, err
	}

	pgInstances, err := agentclient.Instances(ctx, m.appFull.Organization.Slug, m.appFull.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup 6pn ip for %s app: %v", m.appCompact.Name, err)
	}

	if len(pgInstances.Addresses) == 0 {
		return nil, fmt.Errorf("no 6pn ips found for %s app", m.appCompact.Name)
	}

	leaderIP, err := leaderIpFromInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return nil, err
	}

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	stats, err := pgclient.LegacyStolonReplicationStats(ctx)
	if err != nil {
		return nil, err
	}

	res := false
	for _, stat := range stats {
		id := strings.Split(stat.Name, "_")[1]
		for _, dbuid := range dbuids {
			if id == dbuid && stat.Diff == 0 {
				res = true
				return &res, nil
			}
		}
	}
	return &res, nil
}

func (m *v2PlatformMigrator) waitForHealthyPgs(ctx context.Context) error {
	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = m.io.ErrOut
	s.Prefix = "Waiting for in region replicas to become healthy"
	s.Start()

	defer s.Stop()

	timeout := time.After(1 * time.Hour)
	b := &backoff.Backoff{
		Min:    2 * time.Second,
		Max:    5 * time.Minute,
		Factor: 1.2,
		Jitter: true,
	}

	for {
		select {
		case <-time.After(b.Duration()):
			_, err := m.getPgDBUids(ctx, m.inRegionMachines())
			if err == nil {
				return nil
			}
		case <-timeout:
			return errors.New("pgs never got healthy, timing out")
		}
	}
}

func (m *v2PlatformMigrator) waitForPGSync(ctx context.Context, dbuids []string) error {
	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = m.io.ErrOut
	s.Prefix = fmt.Sprintf("Waiting for at least one in region (%s) replica to be synced", m.appConfig.PrimaryRegion)
	s.Start()

	defer s.Stop()
	timeout := time.After(20 * time.Minute)
	ticker := time.Tick(10 * time.Second)
	for {
		select {
		case <-timeout:
			return errors.New("timed out waiting for sync")
		case <-ticker:
			out, err := m.checkPgSync(ctx, dbuids)
			if err != nil {
				return err
			}
			if *out {
				return nil
			}
		}
	}
}
