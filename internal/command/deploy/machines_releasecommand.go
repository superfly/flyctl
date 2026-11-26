package deploy

import (
	"container/ring"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/logs"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const releaseCommandAttemptKey = "fly_release_command_attempt"

var natsConnectTimeout = 10 * time.Second

func (md *machineDeployment) runReleaseCommands(ctx context.Context) error {
	err := md.runReleaseCommand(ctx, "release")

	if err == nil {
		seedCommand := appconfig.SeedCommandFromContext(ctx)

		if seedCommand != "" {
			md.appConfig.Deploy.ReleaseCommand = seedCommand
			err = md.runReleaseCommand(ctx, "seed")
		}
	}

	return err
}

func (md *machineDeployment) runReleaseCommand(ctx context.Context, commandType string) (err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "run_"+commandType+"_cmd")
	defer func() {
		if err != nil {
			tracing.RecordError(ctx, span, err, "failed to run "+commandType+"_cmd")
		}
		span.End()
	}()

	if md.appConfig.Deploy == nil || md.appConfig.Deploy.ReleaseCommand == "" {
		span.AddEvent("no " + commandType + " command")

		return nil
	}

	fmt.Fprintf(md.io.ErrOut, "Running %s %s_command: %s\n",
		md.colorize.Bold(md.app.Name),
		commandType,
		md.appConfig.Deploy.ReleaseCommand,
	)
	ctx, loggerCleanup := statuslogger.SingleLine(ctx, true)
	defer func() {
		if err != nil {
			statuslogger.Failed(ctx, err)
		}
		loggerCleanup(false)
	}()

	logOpts := &logs.LogOptions{
		AppName:    appconfig.NameFromContext(ctx),
		RegionCode: config.FromContext(ctx).Region,
		NoTail:     false,
	}
	var stream logs.LogStream

	eg, groupCtx := errgroup.WithContext(ctx)
	var recoveredStart func(context.Context) error
	eg.Go(func() error {
		var err error
		recoveredStart, err = md.createOrUpdateReleaseCmdMachine(groupCtx)
		if err != nil {
			tracing.RecordError(ctx, span, err, "failed to create "+commandType+" cmd machine")

			return fmt.Errorf("error running %s_command machine: %w", commandType, err)
		}

		return nil
	})
	eg.Go(func() error {
		ctx, cancel := context.WithTimeoutCause(ctx, natsConnectTimeout, fmt.Errorf("connecting release command log stream: %w", context.DeadlineExceeded))
		defer cancel()

		stream, err = logs.NewNatsStream(ctx, md.apiClient, md.flapsClient, logOpts)
		if err != nil {
			// Silently fallback to app logs polling if NATS streaming client is unavailable.
			stream = logs.NewPollingStream(md.apiClient)
		}

		return nil
	})
	if err = eg.Wait(); err != nil {
		return err
	}

	releaseCmdMachine := md.releaseCommandMachine.GetMachines()[0]

	logOpts.VMID = releaseCmdMachine.Machine().ID
	logsCtx, cancelLogs := context.WithCancelCause(ctx)
	defer cancelLogs(fmt.Errorf("release command log streaming stopped: %w", context.Canceled))
	var buf *ring.Ring
	if !flag.GetBool(ctx, "verbose") {
		buf = ring.New(100)
	}
	go func() {
		defer cancelLogs(fmt.Errorf("release command log streaming stopped: %w", context.Canceled))
		if stream == nil {
			return
		}
		for entry := range stream.Stream(logsCtx, logOpts) {
			var ts time.Time
			if ts, err = time.Parse(time.RFC3339Nano, entry.Timestamp); err != nil {
				err = fmt.Errorf("failed parsing timestamp %q: %w", entry.Timestamp, err)

				return
			}
			msg := fmt.Sprintf("%s %s", aurora.Faint(format.Time(ts)), entry.Message)
			if buf != nil {
				buf.Value = msg
				buf = buf.Next()
			} else {
				fmt.Fprintln(md.io.ErrOut)
			}
			if strings.Contains(entry.Message, "Main child exited normally") {
				return
			}
		}
	}()

	fmt.Fprintln(md.io.ErrOut, "Starting machine")

	if recoveredStart == nil {
		err = startReleaseCommandMachine(ctx, func(ctx context.Context) error {
			return releaseCmdMachine.Start(fly.WithoutHTTPRetries(ctx))
		}, md.waitTimeout)
	} else {
		err = recoveredStart(ctx)
	}
	if err != nil {
		fmt.Fprintf(md.io.ErrOut, "error starting %s_command machine: %v\n", commandType, err)

		return
	}

	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the outpu
	err = md.waitForReleaseCommandToFinish(ctx, releaseCmdMachine)
	if err != nil {
		tracing.RecordError(ctx, span, err, "failed to wait for "+commandType+" cmd machine")

		return err
	}
	lastExitEvent, err := releaseCmdMachine.WaitForEventTypeAfterType(ctx, "exit", "start", md.releaseCmdTimeout, true)
	if err != nil {
		return fmt.Errorf("error finding the %s_command machine %s exit event: %w", commandType, releaseCmdMachine.Machine().ID, err)
	}
	exitCode, err := lastExitEvent.Request.GetExitCode()
	if err != nil {
		return fmt.Errorf("error get %s_command machine %s exit code: %w", commandType, releaseCmdMachine.Machine().ID, err)
	}

	if flag.GetBool(ctx, "verbose") {
		waitForLogs(md, logsCtx, stream, releaseCmdMachine.Machine().ID)
	}

	if exitCode != 0 {
		statuslogger.LogStatus(ctx, statuslogger.StatusFailure, commandType+"_command failed")

		// Preemptive cleanup of the logger so that the logs have a clean place to write to
		loggerCleanup(false)

		fmt.Fprintf(md.io.ErrOut, "Error release_command failed running on machine %s with exit code %s.\n",
			md.colorize.Bold(releaseCmdMachine.Machine().ID), md.colorize.Red(strconv.Itoa(exitCode)))

		if !flag.GetBool(ctx, "verbose") {
			fmt.Fprintf(md.io.ErrOut, "Checking logs: fetching the last 100 lines below:\n")
			waitForLogs(md, logsCtx, stream, releaseCmdMachine.Machine().ID)
			buf.Do(func(str any) {
				if str != nil {
					fmt.Fprintln(md.io.ErrOut, str)
				}
			})
		}

		return fmt.Errorf("machine %s exited with non-zero status of %d", releaseCmdMachine.Machine().ID, exitCode)
	}
	statuslogger.LogfStatus(ctx,
		statuslogger.StatusSuccess,
		"%s_command %s completed successfully",
		commandType,
		md.colorize.Bold(releaseCmdMachine.Machine().ID),
	)

	return nil
}

// Wait up to 20 secs to be sure logs have been fully ingested, and log any errors.
func waitForLogs(md *machineDeployment, ctx context.Context, stream logs.LogStream, id string) {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		if fly.IsNotAuthenticatedError(stream.Err()) {
			fmt.Fprintf(md.io.ErrOut, "Warn: not authorized to retrieve app logs (this can happen when using deploy tokens). Use `fly logs -i %s` or open the monitoring dashboard to see them: https://fly.io/apps/%s/monitoring?region=&instance=%s\n", id, md.appConfig.AppName, id)
		} else if stream.Err() != nil && !errors.Is(stream.Err(), context.Canceled) {
			fmt.Fprintf(md.io.ErrOut, "error getting release command logs: %v\n", stream.Err())
		}
	case <-timer.C:
		fmt.Fprintf(md.io.ErrOut, "timeout waiting for release command logs\n")
	}
}

// machineHostDedicationID resolves the host dedication ID from whichever machine
// config is available - the full Config, or the partial IncompleteConfig that's
// all we get for a machine on a non-ok host.
func machineHostDedicationID(m *fly.Machine) string {
	cfg := m.GetConfig()
	if cfg == nil || cfg.Guest == nil {
		return ""
	}

	return cfg.Guest.HostDedicationID
}

// dedicatedHostIdMismatch checks if the dedicatedHostID on a machine is the same as the one set in the fly.toml
// a mismatch will result in a delete+recreate op
func dedicatedHostIdMismatch(m *fly.Machine, ac *appconfig.Config) bool {
	return strings.TrimSpace(ac.HostDedicationID) != "" && machineHostDedicationID(m) != ac.HostDedicationID
}

func (md *machineDeployment) createOrUpdateReleaseCmdMachine(ctx context.Context) (func(context.Context) error, error) {
	span := trace.SpanFromContext(ctx)

	// Existent release command machines must be destroyed if not already, are set to auto-destroy anyways
	if !md.releaseCommandMachine.IsEmpty() {
		mPool := pool.New().WithErrors().WithMaxGoroutines(4).WithContext(ctx)
		for _, m := range md.releaseCommandMachine.GetMachines() {
			mPool.Go(func(ctx context.Context) error {
				return m.Destroy(ctx, true)
			})
		}
		if err := mPool.Wait(); err != nil {
			tracing.RecordError(ctx, span, err, "failed to destroy old release_command machine")
		}
	}

	return md.createReleaseCommandMachine(ctx)
}

func (md *machineDeployment) createReleaseCommandMachine(ctx context.Context) (func(context.Context) error, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "create_release_cmd_machine")
	defer span.End()

	launchInput, err := md.launchInputForReleaseCommand(nil)
	if err != nil {
		return nil, err
	}

	attempt := rand.Text()
	launchInput.Config.Metadata[releaseCommandAttemptKey] = attempt
	// A failed response can arrive after create commits. Issue a single write
	// so the attempt marker identifies only one create request.
	releaseCmdMachine, err := md.flapsClient.Launch(fly.WithoutHTTPRetries(ctx), md.app.Name, *launchInput)
	var recoveredStart func(context.Context) error
	if err != nil {
		var responseErr *flaps.FlapsError
		if errors.As(err, &responseErr) && responseErr.ResponseStatusCode == http.StatusRequestTimeout && md.releaseId != "" && md.waitTimeout > 0 {
			deadline := time.Now().Add(md.waitTimeout)
			waitCtx, cancel := context.WithDeadline(ctx, deadline)
			candidate := md.findReleaseCommandMachine(waitCtx, attempt)
			cancel()
			if candidate != nil {
				recoveredStart = func(ctx context.Context) error {
					recoveryCtx, cancel := context.WithDeadline(ctx, deadline)
					defer cancel()
					return md.startRecoveredReleaseCommandMachine(recoveryCtx, candidate, attempt)
				}
				releaseCmdMachine, err = candidate, nil
			}
		}
		if err != nil {
			tracing.RecordError(ctx, span, err, "failed to create release command machine")
			return nil, fmt.Errorf("error creating a release_command machine: %w", err)
		}
	}

	statuslogger.Logf(ctx, "Created release_command machine %s", md.colorize.Bold(releaseCmdMachine.ID))
	md.releaseCommandMachine = machine.NewMachineSet(md.flapsClient, md.io, md.app.Name, []*fly.Machine{releaseCmdMachine}, true)
	if recoveredStart != nil {
		return recoveredStart, nil
	}

	lm := md.releaseCommandMachine.GetMachines()[0]
	if err := lm.WaitForState(ctx, fly.MachineStateStopped, md.waitTimeout, machine.WithJustCreated()); err != nil {
		return nil, suggestChangeWaitTimeout(err, "wait-timeout")
	}

	return nil, nil
}

func (md *machineDeployment) findReleaseCommandMachine(ctx context.Context, attempt string) *fly.Machine {
	query := url.Values{"metadata." + releaseCommandAttemptKey: {attempt}}.Encode()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var observed *fly.Machine
	for ctx.Err() == nil {
		matches, err := md.flapsClient.List(ctx, md.app.Name, query)
		if err != nil || len(matches) > 1 || ctx.Err() != nil {
			return nil
		}
		if observed != nil && len(matches) == 0 {
			return nil
		}
		if len(matches) == 1 {
			candidate := matches[0]
			if candidate == nil || candidate.ID == "" || candidate.InstanceID == "" {
				return nil
			}
			if observed != nil && (observed.ID != candidate.ID || observed.InstanceID != candidate.InstanceID) {
				return nil
			}
			observed = candidate
			switch candidate.State {
			case fly.MachineStateStopped:
				if (candidate.Config != nil && !md.matchesReleaseCommandConfig(candidate.Config, attempt)) ||
					(len(candidate.Events) > 0 && !hasOnlyCreationEvents(candidate.Events)) {
					return nil
				}
				fresh, err := md.flapsClient.Get(ctx, md.app.Name, candidate.ID)
				if err != nil || ctx.Err() != nil || !md.isUnstartedReleaseCommandMachine(candidate, fresh, attempt) {
					return nil
				}
				return fresh
			case fly.MachineStateCreated, "pending":
				// The build is still preparing the skip-launch Machine.
			default:
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
	return nil
}

func (md *machineDeployment) startRecoveredReleaseCommandMachine(ctx context.Context, candidate *fly.Machine, attempt string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	appName := md.app.Name
	ttl := 120
	// The lease endpoint treats an empty nonce as a new lease. Use the SDK's
	// one-shot POST here because AcquireLease retries ambiguous responses.
	lease, err := md.flapsClient.RefreshLease(fly.WithoutHTTPRetries(ctx), appName, candidate.ID, &ttl, "")
	if err != nil {
		return fmt.Errorf("acquire lease for recovered release_command machine %s: %w", candidate.ID, err)
	}
	if lease == nil || lease.Status != "success" || lease.Data == nil || lease.Data.Nonce == "" {
		return fmt.Errorf("missing lease for recovered release_command machine %s", candidate.ID)
	}
	nonce := lease.Data.Nonce
	releaseLease := true
	defer func() {
		if !releaseLease {
			// An ambiguous Start may still be in flight. Leave its fence in place
			// until the server expires the lease.
			return
		}
		// Release our lease even if the start attempt used the remaining time.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
		defer cancel()
		if err := md.flapsClient.ReleaseLease(fly.WithoutHTTPRetries(cleanupCtx), appName, candidate.ID, nonce); err != nil {
			tracing.RecordError(ctx, trace.SpanFromContext(ctx), err, "failed to release recovered release command machine lease")
		}
	}()
	if lease.Data.Version != candidate.InstanceID {
		return fmt.Errorf("recovered release_command machine %s changed before lease acquisition", candidate.ID)
	}

	// flyd permits a start with any nonce when no lease exists. Send requests
	// only while there is time left on this lease, with room for clock skew.
	leaseCtx, cancel := context.WithDeadline(ctx, time.Unix(lease.Data.ExpiresAt, 0).Add(-5*time.Second))
	defer cancel()
	return startReleaseCommandMachine(leaseCtx, func(ctx context.Context) error {
		fresh, err := md.flapsClient.Get(ctx, appName, candidate.ID)
		if err != nil {
			return fmt.Errorf("check recovered release_command machine %s: %w", candidate.ID, err)
		}
		if !md.isUnstartedReleaseCommandMachine(candidate, fresh, attempt) ||
			!slices.EqualFunc(candidate.Events, fresh.Events, func(before, after *fly.MachineEvent) bool {
				return before.ID == after.ID
			}) {
			return fmt.Errorf("recovered release_command machine %s is not the unstarted Machine from this release", candidate.ID)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := md.flapsClient.Start(fly.WithoutHTTPRetries(ctx), appName, candidate.ID, nonce)
		releaseLease = err == nil || isReleaseCommandBuildNotReady(err)
		if err != nil {
			return err
		}
		if resp == nil || resp.PreviousState != fly.MachineStateStopped {
			return fmt.Errorf("start did not confirm that recovered release_command machine %s was stopped", candidate.ID)
		}
		return nil
	}, md.waitTimeout)
}

func (md *machineDeployment) isUnstartedReleaseCommandMachine(candidate, fresh *fly.Machine, attempt string) bool {
	return fresh != nil && fresh.ID == candidate.ID && fresh.InstanceID == candidate.InstanceID &&
		fresh.State == fly.MachineStateStopped && md.matchesReleaseCommandConfig(fresh.Config, attempt) &&
		hasOnlyCreationEvents(fresh.Events)
}

func (md *machineDeployment) matchesReleaseCommandConfig(cfg *fly.MachineConfig, attempt string) bool {
	return cfg != nil && cfg.AutoDestroy &&
		cfg.Metadata[releaseCommandAttemptKey] == attempt &&
		cfg.Metadata[fly.MachineConfigMetadataKeyFlyReleaseId] == md.releaseId &&
		cfg.ProcessGroup() == fly.MachineProcessGroupFlyAppReleaseCommand
}

func (md *machineDeployment) launchInputForReleaseCommand(origMachineRaw *fly.Machine) (*fly.LaunchMachineInput, error) {
	if origMachineRaw == nil {
		origMachineRaw = &fly.Machine{
			Region: md.appConfig.PrimaryRegion,
		}
	}
	// We can ignore the error because ToReleaseMachineConfig fails only
	// if it can't split the command and we test that at initialization
	mConfig, _ := md.appConfig.ToReleaseMachineConfig()
	mConfig.Image = md.img
	if mConfig.Guest == nil {
		mConfig.Guest = md.inferReleaseCommandGuest()
	}
	md.setMachineReleaseData(mConfig)

	if hdid := md.appConfig.HostDedicationID; hdid != "" {
		mConfig.Guest.HostDedicationID = hdid
	}

	minvers, err := appsecrets.GetMinvers(md.appConfig.AppName)
	if err != nil {
		return nil, err
	}

	return &fly.LaunchMachineInput{
		Config:            mConfig,
		Region:            origMachineRaw.Region,
		SkipLaunch:        true,
		MinSecretsVersion: minvers,
	}, nil
}

func (md *machineDeployment) inferReleaseCommandGuest() *fly.MachineGuest {
	defaultGuest := fly.MachinePresets[fly.DefaultVMSize]
	desiredGuest := fly.MachinePresets["shared-cpu-2x"]
	if mg := md.machineGuest; mg != nil && (mg.CPUKind != defaultGuest.CPUKind || mg.CPUs != defaultGuest.CPUs || mg.MemoryMB != defaultGuest.MemoryMB) {
		desiredGuest = mg
	}
	if !md.machineSet.IsEmpty() {
		group := md.appConfig.DefaultProcessName()
		ram := func(m *fly.Machine) int {
			if m != nil && m.Config != nil && m.Config.Guest != nil {
				return m.Config.Guest.MemoryMB
			}

			return 0
		}

		maxRamMach := lo.Reduce(md.machineSet.GetMachines(), func(prevBest *fly.Machine, lm machine.LeasableMachine, _ int) *fly.Machine {
			mach := lm.Machine()
			if mach.ProcessGroup() != group {
				return prevBest
			}

			return lo.Ternary(ram(mach) > ram(prevBest), mach, prevBest)
		}, nil)
		if maxRamMach != nil {
			desiredGuest = maxRamMach.Config.Guest
		}
	}

	return helpers.Clone(desiredGuest)
}

// A skip-launch build can report stopped before its final writes finish. flyd
// waits briefly, then explicitly refuses Start so the caller can retry safely.
func startReleaseCommandMachine(ctx context.Context, start func(context.Context) error, timeout time.Duration) error {
	// Match Machine.WaitForState: zero uses a bounded two-minute fallback.
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	retryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastRefusal error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if lastRefusal != nil && retryCtx.Err() != nil {
			return fmt.Errorf("machine build still not ready after %s (--wait-timeout): %w", timeout, lastRefusal)
		}
		// Bound further attempts, not an accepted Start. The caller still owns cancellation.
		err := start(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isReleaseCommandBuildNotReady(err) {
			return err
		}
		lastRefusal = err

		timer := time.NewTimer(time.Second)
		select {
		case <-retryCtx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

func isReleaseCommandBuildNotReady(err error) bool {
	var flapsErr *flaps.FlapsError
	return errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusPreconditionFailed &&
		flapsErr.Error() == "failed_precondition: machine build not ready, retry the start request"
}

func (md *machineDeployment) waitForReleaseCommandToFinish(ctx context.Context, releaseCmdMachine machine.LeasableMachine) error {
	err := releaseCmdMachine.WaitForState(ctx, fly.MachineStateStarted, md.waitTimeout)
	if err != nil {
		var flapsErr *flaps.FlapsError
		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusNotFound {
			// The machine exited and was destroyed quickly.
			return nil
		}
		err = suggestChangeWaitTimeout(err, "wait-timeout")

		return fmt.Errorf("error waiting for release_command machine %s to start: %w", releaseCmdMachine.Machine().ID, err)
	}
	err = releaseCmdMachine.WaitForState(ctx, fly.MachineStateDestroyed, md.releaseCmdTimeout, machine.WithAllowInfinite(true))
	if err != nil {
		err = suggestChangeWaitTimeout(err, "release-command-timeout")

		return fmt.Errorf("error waiting for release_command machine %s to finish running: %w", releaseCmdMachine.Machine().ID, err)
	}
	md.releaseCommandMachine.RemoveMachines(ctx, []machine.LeasableMachine{releaseCmdMachine})

	return nil
}

func hasOnlyCreationEvents(events []*fly.MachineEvent) bool {
	expected := [][2]string{{"update", "stopped"}, {"launch", "created"}, {"launch", "pending"}}
	if len(events) < 2 || len(events) > len(expected) {
		return false
	}
	ids := map[string]bool{}
	for i, e := range events {
		if e == nil || e.ID == "" || ids[e.ID] || e.Type != expected[i][0] || e.Status != expected[i][1] {
			return false
		}
		ids[e.ID] = true
	}
	return true
}
