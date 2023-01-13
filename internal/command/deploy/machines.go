package deploy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/jpillora/backoff"
	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

const defaultLeaseTtl = 10 * time.Minute
const defaultWaitTimeout = 120 * time.Second

// FIXME: move a lot of this stuff to internal/machine pkg... maybe all of it?
type MachineDeployment interface {
	DeployMachinesApp(context.Context) error
}

type MachineDeploymentArgs struct {
	Strategy             string
	AutoConfirmMigration bool
	SkipHealthChecks     bool
	RestartOnly          bool
}

type machineDeployment struct {
	gqlClient                  graphql.Client
	flapsClient                *flaps.Client
	io                         *iostreams.IOStreams
	colorize                   *iostreams.ColorScheme
	app                        *api.AppCompact
	appConfig                  *app.Config
	img                        *imgsrc.DeploymentImage
	machineSet                 MachineSet
	releaseCommandMachine      MachineSet
	releaseCommand             string
	volumeName                 string
	volumeDestination          string
	strategy                   string
	releaseId                  string
	releaseVersion             int
	autoConfirmAppsV2Migration bool
	skipHealthChecks           bool
	restartOnly                bool
}

type MachineSet interface {
	AcquireLeases(context.Context, time.Duration) error
	ReleaseLeases(context.Context) error
	IsEmpty() bool
	GetMachines() []LeasableMachine
}

type machineSet struct {
	machines []LeasableMachine
}

type LeasableMachine interface {
	Machine() *api.Machine
	HasLease() bool
	AcquireLease(context.Context, time.Duration) error
	ReleaseLease(context.Context) error
	Update(context.Context, api.LaunchMachineInput) error
	Start(context.Context) error
	WaitForState(context.Context, string, time.Duration) error
	WaitForHealthchecksToPass(context.Context, time.Duration) error
	WaitForEventTypeAfterType(context.Context, string, string, time.Duration) (*api.MachineEvent, error)
}

type leasableMachine struct {
	flapsClient *flaps.Client
	io          *iostreams.IOStreams
	colorize    *iostreams.ColorScheme

	lock            sync.RWMutex
	machine         *api.Machine
	leaseNonce      string
	leaseExpiration time.Time
}

func NewLeasableMachine(flapsClient *flaps.Client, io *iostreams.IOStreams, machine *api.Machine) LeasableMachine {
	return &leasableMachine{
		flapsClient: flapsClient,
		io:          io,
		colorize:    io.ColorScheme(),
		machine:     machine,
	}
}

func (lm *leasableMachine) Update(ctx context.Context, input api.LaunchMachineInput) error {
	if !lm.HasLease() {
		return fmt.Errorf("no current lease for machine %s", lm.machine.ID)
	}
	lm.lock.Lock()
	defer lm.lock.Unlock()
	updateMachine, err := lm.flapsClient.Update(ctx, input, lm.leaseNonce)
	if err != nil {
		return err
	}
	lm.machine = updateMachine
	return nil
}

func (md *machineDeployment) logClearLinesAbove(count int) {
	if md.io.IsInteractive() {
		builder := aec.EmptyBuilder
		str := builder.Up(uint(count)).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(md.io.ErrOut, str.String())
	}
}

func (lm *leasableMachine) logClearLinesAbove(count int) {
	if lm.io.IsInteractive() {
		builder := aec.EmptyBuilder
		str := builder.Up(uint(count)).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(lm.io.ErrOut, str.String())
	}
}

func (lm *leasableMachine) logStatus(desired, current string) {
	cur := lm.colorize.Green(current)
	if desired != current {
		cur = lm.colorize.Yellow(current)
	}
	fmt.Fprintf(lm.io.ErrOut, "  Waiting for %s to have state %s, currently: %s\n",
		lm.colorize.Bold(lm.Machine().ID),
		lm.colorize.Green(desired),
		cur,
	)
}

func (lm *leasableMachine) logHealthCheckStatus(status *api.HealthCheckStatus) {
	if status == nil {
		return
	}
	resColor := lm.colorize.Green
	if status.Passing != status.Total {
		resColor = lm.colorize.Yellow
	}
	fmt.Fprintf(lm.io.ErrOut, "  Waiting for %s to become healthy: %s\n",
		lm.colorize.Bold(lm.Machine().ID),
		resColor(fmt.Sprintf("%d/%d", status.Passing, status.Total)),
	)
}

func (lm *leasableMachine) Start(ctx context.Context) error {
	if lm.HasLease() {
		return fmt.Errorf("error cannot start machine %s because it has a lease expiring at %s", lm.machine.ID, lm.leaseExpiration.Format(time.RFC3339))
	}
	lm.lock.RLock()
	defer lm.lock.RUnlock()
	lm.logStatus(api.MachineStateStarted, lm.machine.State)
	_, err := lm.flapsClient.Start(ctx, lm.machine.ID)
	if err != nil {
		return err
	}
	return nil
}

func (lm *leasableMachine) WaitForState(ctx context.Context, desiredState string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	for {
		err := lm.flapsClient.Wait(waitCtx, lm.Machine(), desiredState, timeout)
		switch {
		case errors.Is(err, context.Canceled):
			return err
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("timeout reached waiting for machine to %s %w", desiredState, err)
		case err != nil:
			if lm.io.IsInteractive() {
				updatedMachine, err := lm.flapsClient.Get(ctx, lm.machine.ID)
				if err == nil && updatedMachine != nil {
					lm.logClearLinesAbove(1)
					lm.logStatus(desiredState, updatedMachine.State)
				}
			}
			time.Sleep(b.Duration())
			continue
		}
		if lm.io.IsInteractive() {
			lm.logClearLinesAbove(1)
		}
		lm.logStatus(desiredState, desiredState)
		return nil
	}
}

func (lm *leasableMachine) WaitForHealthchecksToPass(ctx context.Context, timeout time.Duration) error {
	if lm.machine.Config.Checks == nil {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	shortestInterval := 120 * time.Second
	for _, c := range lm.Machine().Config.Checks {
		ci := c.Interval.Duration
		if ci < shortestInterval {
			shortestInterval = ci
		}
	}
	b := &backoff.Backoff{
		Min:    shortestInterval / 2,
		Max:    2 * shortestInterval,
		Factor: 2,
		Jitter: true,
	}
	printedFirst := false
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(err, context.Canceled):
			return err
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("timeout reached waiting for healthchecks to pass for machine %s %w", lm.Machine().ID, err)
		case err != nil:
			return fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		case !updateMachine.HealthCheckStatus().AllPassing():
			if !printedFirst || lm.io.IsInteractive() {
				lm.logClearLinesAbove(1)
				lm.logHealthCheckStatus(updateMachine.HealthCheckStatus())
				printedFirst = true
			}
			time.Sleep(b.Duration())
			continue
		}
		lm.logClearLinesAbove(1)
		lm.logHealthCheckStatus(updateMachine.HealthCheckStatus())
		return nil
	}
}

// waits for an eventType1 type event to show up after we see a eventType2 event, and returns it
func (lm *leasableMachine) WaitForEventTypeAfterType(ctx context.Context, eventType1, eventType2 string, timeout time.Duration) (*api.MachineEvent, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	lm.logClearLinesAbove(1)
	fmt.Fprintf(lm.io.ErrOut, "  Waiting for %s to get %s event\n",
		lm.colorize.Bold(lm.Machine().ID),
		lm.colorize.Yellow(eventType1),
	)
	for {
		updateMachine, err := lm.flapsClient.Get(waitCtx, lm.Machine().ID)
		switch {
		case errors.Is(err, context.Canceled):
			return nil, err
		case errors.Is(err, context.DeadlineExceeded):
			return nil, fmt.Errorf("timeout reached waiting for healthchecks to pass for machine %s %w", lm.Machine().ID, err)
		case err != nil:
			return nil, fmt.Errorf("error getting machine %s from api: %w", lm.Machine().ID, err)
		}
		exitEvent := updateMachine.GetLatestEventOfTypeAfterType(eventType1, eventType2)
		if exitEvent != nil {
			return exitEvent, nil
		} else {
			time.Sleep(b.Duration())
		}
	}
}

func (lm *leasableMachine) Machine() *api.Machine {
	lm.lock.RLock()
	defer lm.lock.RUnlock()
	return lm.machine
}

func (lm *leasableMachine) HasLease() bool {
	lm.lock.RLock()
	defer lm.lock.RUnlock()
	return lm.leaseNonce != "" && lm.leaseExpiration.After(time.Now())
}

func (lm *leasableMachine) AcquireLease(ctx context.Context, duration time.Duration) error {
	if lm.HasLease() {
		return nil
	}
	seconds := int(duration.Seconds())
	lease, err := lm.flapsClient.AcquireLease(ctx, lm.machine.ID, &seconds)
	if err != nil {
		return err
	}
	if lease.Status != "success" {
		return fmt.Errorf("did not acquire lease for machine %s status: %s code: %s message: %s", lm.machine.ID, lease.Status, lease.Code, lease.Message)
	}
	if lease.Data == nil {
		return fmt.Errorf("missing data from lease response for machine %s, assuming not successful", lm.machine.ID)
	}
	lm.lock.Lock()
	defer lm.lock.Unlock()
	lm.leaseNonce = lease.Data.Nonce
	lm.leaseExpiration = time.Unix(lease.Data.ExpiresAt, 0)
	return nil
}

func (lm *leasableMachine) ReleaseLease(ctx context.Context) error {
	if !lm.HasLease() {
		lm.resetLease()
		return nil
	}
	// don't bother releasing expired leases, and allow for some clock skew between flyctl and flaps
	if time.Since(lm.leaseExpiration) > 5*time.Second {
		lm.resetLease()
		return nil
	}
	err := lm.flapsClient.ReleaseLease(ctx, lm.machine.ID, lm.leaseNonce)
	if err != nil {
		terminal.Warnf("failed to release lease for machine %s (expires at %s): %v\n", lm.machine.ID, lm.leaseExpiration.Format(time.RFC3339), err)
		lm.resetLease()
		return err
	}
	lm.resetLease()
	return nil
}

func (lm *leasableMachine) resetLease() {
	lm.lock.Lock()
	defer lm.lock.Unlock()
	lm.leaseNonce = ""
}

func NewMachineSet(flapsClient *flaps.Client, io *iostreams.IOStreams, machines []*api.Machine) MachineSet {
	leaseMachines := make([]LeasableMachine, 0)
	for _, m := range machines {
		leaseMachines = append(leaseMachines, NewLeasableMachine(flapsClient, io, m))
	}
	return &machineSet{
		machines: leaseMachines,
	}
}

func (ms *machineSet) IsEmpty() bool {
	return len(ms.machines) == 0
}

func (ms *machineSet) GetMachines() []LeasableMachine {
	return ms.machines
}

func (ms *machineSet) AcquireLeases(ctx context.Context, duration time.Duration) error {
	results := make(chan error, len(ms.machines))
	var wg sync.WaitGroup
	for _, m := range ms.machines {
		wg.Add(1)
		go func(m LeasableMachine) {
			defer wg.Done()
			results <- m.AcquireLease(ctx, duration)
		}(m)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	hadError := false
	for err := range results {
		if err != nil {
			hadError = true
			terminal.Warnf("failed to acquire lease: %v\n", err)
		}
	}
	if hadError {
		if err := ms.ReleaseLeases(ctx); err != nil {
			terminal.Warnf("error releasing machine leases: %v\n", err)
		}
		return fmt.Errorf("error acquiring leases on all machines")
	}
	return nil
}

func (ms *machineSet) ReleaseLeases(ctx context.Context) error {
	// when context is canceled, take 500ms to attempt to release the leases
	contextWasAlreadyCanceled := errors.Is(ctx.Err(), context.Canceled)
	if contextWasAlreadyCanceled {
		var cancel context.CancelFunc
		cancelTimeout := 500 * time.Millisecond
		ctx, cancel = context.WithTimeout(context.TODO(), cancelTimeout)
		terminal.Infof("detected canceled context and allowing %s to release machine leases\n", cancelTimeout)
		defer cancel()
	}

	results := make(chan error, len(ms.machines))
	var wg sync.WaitGroup
	for _, m := range ms.machines {
		wg.Add(1)
		go func(m LeasableMachine) {
			defer wg.Done()
			results <- m.ReleaseLease(ctx)
		}(m)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	hadError := false
	for err := range results {
		contextTimedOutOrCanceled := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
		if err != nil && (!contextWasAlreadyCanceled || !contextTimedOutOrCanceled) {
			hadError = true
			terminal.Warnf("failed to release lease: %v\n", err)
		}
	}
	if hadError {
		return fmt.Errorf("error releasing leases on machines")
	}
	return nil
}

func NewMachineDeployment(ctx context.Context, args MachineDeploymentArgs) (MachineDeployment, error) {
	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		return nil, err
	}
	err = appConfig.Validate()
	if err != nil {
		return nil, err
	}
	app, err := client.FromContext(ctx).API().GetAppCompact(ctx, appConfig.AppName)
	if err != nil {
		return nil, err
	}
	// FIXME: don't call this here... it rebuilds everything... we'll have to pass it in I guess?
	img, err := determineImage(ctx, appConfig)
	if err != nil {
		return nil, err
	}
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return nil, err
	}
	releaseCmd := ""
	if appConfig.Deploy != nil {
		releaseCmd = appConfig.Deploy.ReleaseCommand
	}
	io := iostreams.FromContext(ctx)
	md := &machineDeployment{
		gqlClient:                  client.FromContext(ctx).API().GenqClient,
		flapsClient:                flapsClient,
		io:                         io,
		colorize:                   io.ColorScheme(),
		app:                        app,
		appConfig:                  appConfig,
		img:                        img,
		autoConfirmAppsV2Migration: args.AutoConfirmMigration,
		skipHealthChecks:           args.SkipHealthChecks,
		restartOnly:                args.RestartOnly,
		releaseCommand:             releaseCmd,
	}
	md.setStrategy(args.Strategy)
	err = md.setVolumeConfig()
	if err != nil {
		return nil, err
	}
	err = md.setMachinesForDeployment(ctx)
	if err != nil {
		return nil, err
	}
	err = md.validateVolumeConfig()
	if err != nil {
		return nil, err
	}
	err = md.createReleaseInBackend(ctx)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func (md *machineDeployment) runReleaseCommand(ctx context.Context) error {
	if md.releaseCommand == "" || md.releaseCommandMachine.IsEmpty() || md.restartOnly {
		return nil
	}
	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.ErrOut, "Running release command: %s\n", md.appConfig.Deploy.ReleaseCommand)
	err := md.createOrUpdateReleaseCmdMachine(ctx)
	if err != nil {
		return fmt.Errorf("error running release_command machine: %w", err)
	}
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the outpu
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateStarted, defaultWaitTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for release_command machine %s to start: %w", releaseCmdMachine.Machine().ID, err)
	}
	err = releaseCmdMachine.WaitForState(ctx, api.MachineStateStopped, defaultWaitTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for release_command machine %s to finish running: %w", releaseCmdMachine.Machine().ID, err)
	}
	lastExitEvent, err := releaseCmdMachine.WaitForEventTypeAfterType(ctx, "exit", "start", defaultWaitTimeout)
	if err != nil {
		return fmt.Errorf("error finding the release_command machine %s exit event: %w", releaseCmdMachine.Machine().ID, err)
	}
	exitCode, err := lastExitEvent.Request.GetExitCode()
	if err != nil {
		return fmt.Errorf("error get release_command machine %s exit code: %w", releaseCmdMachine.Machine().ID, err)
	}
	if exitCode != 0 {
		fmt.Fprintf(md.io.ErrOut, "Error release_command failed running on machine %s with exit code %s. Check the logs at: https://fly.io/apps/%s/monitoring\n",
			md.colorize.Bold(releaseCmdMachine.Machine().ID), md.colorize.Red(strconv.Itoa(exitCode)), md.app.Name)
		return fmt.Errorf("error release_command machine %s exited with non-zero status of %d", releaseCmdMachine.Machine().ID, exitCode)
	}
	md.logClearLinesAbove(1)
	fmt.Fprintf(md.io.ErrOut, "  release_command %s completed successfully\n", md.colorize.Bold(releaseCmdMachine.Machine().ID))
	return nil
}

func (md *machineDeployment) DeployMachinesApp(ctx context.Context) error {
	err := md.runReleaseCommand(ctx)
	if err != nil {
		return fmt.Errorf("release command failed - aborting deployment. %w", err)
	}

	io := iostreams.FromContext(ctx)
	ctx = flaps.NewContext(ctx, md.flapsClient)

	regionCode := md.appConfig.PrimaryRegion

	machineConfig := md.baseMachineConfig(api.MachineProcessGroupApp)
	machineConfig.Metadata["process_group"] = api.MachineProcessGroupApp
	machineConfig.Init.Cmd = nil

	if !md.machineSet.IsEmpty() {

		// FIXME: consolidate all this config stuff into a md.ResolveConfig() or something like that, and deal with restartOnly there

		err := md.machineSet.AcquireLeases(ctx, defaultLeaseTtl)
		defer func() {
			err := md.machineSet.ReleaseLeases(ctx)
			if err != nil {
				terminal.Warnf("error releasing leases on machines: %v\n", err)
			}
		}()
		if err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Deploying %s app with %s strategy\n", md.colorize.Bold(md.app.Name), md.strategy)

		// FIXME: handle deploy strategy: rolling, immediate, canary, bluegreen

		for _, m := range md.machineSet.GetMachines() {
			machine := m.Machine()
			launchInput := api.LaunchMachineInput{
				ID:      machine.ID,
				AppID:   md.app.Name,
				OrgSlug: md.app.Organization.ID,
				Config:  machineConfig,
				Region:  regionCode,
			}

			if md.restartOnly {
				launchInput.Config = machine.Config
				// FIXME: should we skip over all the other config stuff?
			}

			launchInput.Region = machine.Region

			machineConfig.Metadata = machine.Config.Metadata

			if machineConfig.Metadata == nil {
				machineConfig.Metadata = map[string]string{
					"process_group": "app",
				}
			}

			if md.app.IsPostgresApp() {
				machineConfig.Metadata["fly-managed-postgres"] = "true"
			}

			if launchInput.Config.Env["PRIMARY_REGION"] == "" {
				if launchInput.Config.Env == nil {
					launchInput.Config.Env = map[string]string{}
				}
				launchInput.Config.Env["PRIMARY_REGION"] = machine.Config.Env["PRIMARY_REGION"]
			}

			// FIXME: this should just come from appConfig, right? we want folks to configure [checks] to manage these
			launchInput.Config.Checks = machine.Config.Checks

			// FIXME: this should be set from the appConfig, right? in particular this ensures all the machines have the same cpu, mem, etc
			if machine.Config.Guest != nil {
				launchInput.Config.Guest = machine.Config.Guest
			}

			if machine.Config.Mounts != nil {
				launchInput.Config.Mounts = machine.Config.Mounts
			}
			if len(launchInput.Config.Mounts) == 1 && launchInput.Config.Mounts[0].Path != md.volumeDestination {
				currentMount := launchInput.Config.Mounts[0]
				terminal.Warnf("Updating the mount path for volume %s on machine %s from %s to %s due to fly.toml [mounts] destination value\n", currentMount.Volume, machine.ID, currentMount.Path, md.volumeDestination)
				launchInput.Config.Mounts[0].Path = md.volumeDestination
			}

			fmt.Fprintf(io.ErrOut, "  Updating %s\n", md.colorize.Bold(m.Machine().ID))
			err := m.Update(ctx, launchInput)
			if err != nil {
				if md.strategy != "immediate" {
					return err
				} else {
					fmt.Printf("Continuing after error: %s\n", err)
				}
			}

			if md.strategy != "immediate" {
				err := m.WaitForState(ctx, api.MachineStateStarted, defaultWaitTimeout)
				if err != nil {
					return err
				}
			}

			if md.strategy != "immediate" && !md.skipHealthChecks {
				err := m.WaitForHealthchecksToPass(ctx, defaultWaitTimeout)
				// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
				if err != nil {
					return err
				}
			}
		}

		fmt.Fprintf(io.ErrOut, "  Finished deploying\n")

	} else {
		// FIXME: what do we want to do when no machines are present? Launch one? maybe only do that if there aren't other machines in the app, at all?
		return fmt.Errorf("NOPE NOPE NOPE: not implemented yet :-(")
		// fmt.Fprintf(io.Out, "Launching VM with image %s\n", launchInput.Config.Image)
		// _, err = md.flapsClient.Launch(ctx, launchInput)
		// if err != nil {
		// 	return err
		// }
	}

	return nil
}

func (md *machineDeployment) setMachinesForDeployment(ctx context.Context) error {
	machines, releaseCmdMachine, err := md.flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	// migrate non-platform machines into fly platform
	if len(machines) == 0 {
		terminal.Debug("Found no machines that are part of Fly Apps Platform. Check for other machines...")
		machines, err = md.flapsClient.ListActive(ctx)
		if err != nil {
			return err
		}
		if len(machines) > 0 {
			rows := make([][]string, 0)
			for _, machine := range machines {
				var volName string
				if machine.Config != nil && len(machine.Config.Mounts) > 0 {
					volName = machine.Config.Mounts[0].Volume
				}

				rows = append(rows, []string{
					machine.ID,
					machine.Name,
					machine.State,
					machine.Region,
					machine.ImageRefWithVersion(),
					machine.PrivateIP,
					volName,
					machine.CreatedAt,
					machine.UpdatedAt,
				})
			}
			terminal.Warnf("Found %d machines that are not part of the Fly Apps Platform:\n", len(machines))
			_ = render.Table(iostreams.FromContext(ctx).Out, fmt.Sprintf("%s machines", md.app.Name), rows, "ID", "Name", "State", "Region", "Image", "IP Address", "Volume", "Created", "Last Updated")
			if !md.autoConfirmAppsV2Migration {
				switch confirmed, err := prompt.Confirmf(ctx, "Migrate %d existing machines into Fly Apps Platform?", len(machines)); {
				case err == nil:
					if !confirmed {
						terminal.Info("Skipping machines migration to Fly Apps Platform and the deployment")
						md.machineSet = NewMachineSet(md.flapsClient, md.io, nil)
						return nil
					}
				case prompt.IsNonInteractive(err):
					return prompt.NonInteractiveError("not running interactively, use --auto-confirm flag to confirm")
				default:
					return err
				}
			}
			terminal.Infof("Migrating %d machines to the Fly Apps Platform\n", len(machines))
		}
	}

	md.machineSet = NewMachineSet(md.flapsClient, md.io, machines)
	var releaseCmdSet []*api.Machine
	if releaseCmdMachine != nil {
		releaseCmdSet = []*api.Machine{releaseCmdMachine}
	}
	md.releaseCommandMachine = NewMachineSet(md.flapsClient, md.io, releaseCmdSet)
	return nil
}

func (md *machineDeployment) createOrUpdateReleaseCmdMachine(ctx context.Context) error {
	if md.releaseCommandMachine.IsEmpty() {
		err := md.createReleaseCommandMachine(ctx)
		if err != nil {
			return err
		}
	} else {
		err := md.updateReleaseCommandMachine(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (md *machineDeployment) createReleaseCommandMachine(ctx context.Context) error {
	if md.releaseCommand == "" || !md.releaseCommandMachine.IsEmpty() {
		return nil
	}
	machineConf := md.baseMachineConfig(api.MachineProcessGroupReleaseCommand)
	machineConf.Init.Cmd = strings.Split(md.releaseCommand, " ")
	machineConf.Services = nil
	machineConf.Restart = api.MachineRestart{
		Policy: api.MachineRestartPolicyNo,
	}
	// FIXME: do we need to set volumes on release_command machines? I think no?
	// FIXME: figure out how to set cpu/mem/vmclass for release_command machine
	updatedConfig := api.LaunchMachineInput{
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  machineConf,
	}
	if md.appConfig.PrimaryRegion != "" {
		updatedConfig.Region = md.appConfig.PrimaryRegion
	}
	releaseCmdMachine, err := md.flapsClient.Launch(ctx, updatedConfig)
	if err != nil {
		return fmt.Errorf("error creating a release_command machine: %w", err)
	}
	md.releaseCommandMachine = NewMachineSet(md.flapsClient, md.io, []*api.Machine{releaseCmdMachine})
	return nil
}

func (md *machineDeployment) updateReleaseCommandMachine(ctx context.Context) error {
	if md.releaseCommand == "" {
		return nil
	}
	if md.releaseCommandMachine.IsEmpty() {
		return fmt.Errorf("expected release_command machine to exist already, but it does not :-(")
	}
	io := iostreams.FromContext(ctx)
	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]
	fmt.Fprintf(io.ErrOut, "Updating release command machine %s in preparation to run later\n", releaseCmdMachine.Machine().ID)
	err := releaseCmdMachine.WaitForState(ctx, api.MachineStateStopped, defaultWaitTimeout)
	if err != nil {
		return err
	}
	err = md.releaseCommandMachine.AcquireLeases(ctx, defaultLeaseTtl)
	defer func() {
		_ = md.releaseCommandMachine.ReleaseLeases(ctx)
	}()
	if err != nil {
		return err
	}
	machineConf := md.baseMachineConfig(api.MachineProcessGroupReleaseCommand)
	machineConf.Init.Cmd = strings.Split(md.releaseCommand, " ")
	machineConf.Services = nil
	machineConf.Restart = api.MachineRestart{
		Policy: api.MachineRestartPolicyNo,
	}
	// FIXME: do we need to set volumes on release_command machines? I think no?
	// FIXME: figure out how to set cpu/mem/vmclass for release_command machine
	updatedConfig := api.LaunchMachineInput{
		ID:      releaseCmdMachine.Machine().ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  machineConf,
		Region:  releaseCmdMachine.Machine().Region,
	}
	err = releaseCmdMachine.Update(ctx, updatedConfig)
	if err != nil {
		return fmt.Errorf("error updating release_command machine: %w", err)
	}
	return nil
}

func (md *machineDeployment) setVolumeConfig() error {
	if md.appConfig.Mounts != nil {
		md.volumeName = md.appConfig.Mounts.Source
		md.volumeDestination = md.appConfig.Mounts.Destination
	}
	return nil
}

func (md *machineDeployment) validateVolumeConfig() error {
	if md.machineSet.IsEmpty() {
		return nil
	}
	for _, m := range md.machineSet.GetMachines() {
		mid := m.Machine().ID
		mountsConfig := m.Machine().Config.Mounts
		if len(mountsConfig) > 1 {
			return fmt.Errorf("error machine %s has %d mounts and expected 1", mid, len(mountsConfig))
		}
		if md.volumeName == "" {
			if len(mountsConfig) != 0 {
				return fmt.Errorf("error machine %s has a volume mounted and app config does not specify a volume; remove the volume from the machine or add a [mounts] configuration to fly.toml", mid)
			}
		} else {
			if len(mountsConfig) == 0 {
				return fmt.Errorf("error machine %s does not have a volume configured and fly.toml expects one with name %s; remove the [mounts] configuration in fly.toml or use the machines API to add a volume to this machine", mid, md.volumeName)
			}
			mVolName := mountsConfig[0].Name
			if md.volumeName != mVolName {
				return fmt.Errorf("error machine %s has volume with name %s and fly.toml has [mounts] source set to %s; update the source to %s or use the machines API to attach a volume with name %s to this machine", mid, mVolName, md.volumeName, mVolName, md.volumeName)
			}
		}
	}
	return nil
}

func (md *machineDeployment) setStrategy(passedInStrategy string) {
	if passedInStrategy != "" {
		md.strategy = passedInStrategy
		return
	}
	stratFromConfig := md.appConfig.GetDeployStrategy()
	if stratFromConfig != "" {
		md.strategy = stratFromConfig
		return
	}
	// FIXME: any other checks we want to do here? e.g., we used to do canary if any max_per_region==0 && app.distinct_regions?==false
	md.strategy = "rolling"
}

func (md *machineDeployment) createReleaseInBackend(ctx context.Context) error {
	_ = `# @genqlient
	mutation MachinesCreateRelease($input:CreateReleaseInput!) {
		createRelease(input:$input) {
			release {
				id
				version
			}
		}
	}
	`
	input := gql.CreateReleaseInput{
		AppId:           md.appConfig.AppName,
		Image:           md.img.Tag,
		PlatformVersion: "machines",
		Strategy:        gql.DeploymentStrategy(strings.ToUpper(md.strategy)),
		Definition:      md.appConfig.Definition,
	}
	resp, err := gql.MachinesCreateRelease(ctx, md.gqlClient, input)
	if err != nil {
		return err
	}
	md.releaseId = resp.CreateRelease.Release.Id
	md.releaseVersion = resp.CreateRelease.Release.Version
	return nil
}

func (md *machineDeployment) baseMachineConfig(processGroup string) *api.MachineConfig {
	machineConfig := &api.MachineConfig{}
	machineConfig.Metadata = map[string]string{
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyReleaseId:       md.releaseId,
		api.MachineConfigMetadataKeyFlyReleaseVersion:  strconv.Itoa(md.releaseVersion),
		api.MachineConfigMetadataKeyProcessGroup:       processGroup,
	}

	if md.restartOnly {
		return machineConfig
	}

	machineConfig.Image = md.img.Tag

	// Convert the new, slimmer http service config to standard services
	if md.appConfig.HttpService != nil {
		concurrency := md.appConfig.HttpService.Concurrency

		if concurrency != nil {
			if concurrency.Type == "" {
				concurrency.Type = "requests"
			}
			if concurrency.HardLimit == 0 {
				concurrency.HardLimit = 25
			}
			if concurrency.SoftLimit == 0 {
				concurrency.SoftLimit = int(math.Ceil(float64(concurrency.HardLimit) * 0.8))
			}
		}

		httpService := api.MachineService{
			Protocol:     "tcp",
			InternalPort: md.appConfig.HttpService.InternalPort,
			Concurrency:  concurrency,
			Ports: []api.MachinePort{
				{
					Port:       api.IntPointer(80),
					Handlers:   []string{"http"},
					ForceHttps: md.appConfig.HttpService.ForceHttps,
				},
				{
					Port:     api.IntPointer(443),
					Handlers: []string{"http", "tls"},
				},
			},
		}

		machineConfig.Services = append(machineConfig.Services, httpService)
	}

	// Copy standard services to the machine vonfig
	if md.appConfig.Services != nil {
		machineConfig.Services = append(machineConfig.Services, md.appConfig.Services...)
	}

	if md.appConfig.Env != nil {
		machineConfig.Env = md.appConfig.Env
	}

	if md.appConfig.Metrics != nil {
		machineConfig.Metrics = md.appConfig.Metrics
	}

	if md.appConfig.Checks != nil {
		machineConfig.Checks = md.appConfig.Checks
	}

	return machineConfig
}
