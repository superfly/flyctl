package deploy

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"net/http"
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
			tracing.RecordError(span, err, "failed to run "+commandType+"_cmd")
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
	eg.Go(func() error {
		err := md.createOrUpdateReleaseCmdMachine(groupCtx)
		if err != nil {
			tracing.RecordError(span, err, "failed to create "+commandType+" cmd machine")
			return fmt.Errorf("error running %s_command machine: %w", commandType, err)
		}
		return nil
	})
	eg.Go(func() error {
		stream, err = logs.NewNatsStream(ctx, md.apiClient, logOpts)
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
	logsCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()
	var buf *ring.Ring
	if !flag.GetBool(ctx, "verbose") {
		buf = ring.New(100)
	}
	go func() {
		defer cancelLogs()
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

	if err = releaseCmdMachine.Start(ctx); err != nil {
		fmt.Fprintf(md.io.ErrOut, "error starting %s_command machine: %v\n", commandType, err)
		return
	}

	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the outpu
	err = md.waitForReleaseCommandToFinish(ctx, releaseCmdMachine)
	if err != nil {
		tracing.RecordError(span, err, "failed to wait for "+commandType+" cmd machine")

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

// dedicatedHostIdMismatch checks if the dedicatedHostID on a machine is the same as the one set in the fly.toml
// a mismatch will result in a delete+recreate op
func dedicatedHostIdMismatch(m *fly.Machine, ac *appconfig.Config) bool {
	return strings.TrimSpace(ac.HostDedicationID) != "" && m.Config.Guest.HostDedicationID != ac.HostDedicationID
}

func (md *machineDeployment) createOrUpdateReleaseCmdMachine(ctx context.Context) error {
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
			tracing.RecordError(span, err, "failed to destroy old release_command machine")
		}
	}

	return md.createReleaseCommandMachine(ctx)
}

func (md *machineDeployment) createReleaseCommandMachine(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "create_release_cmd_machine")
	defer span.End()

	launchInput := md.launchInputForReleaseCommand(nil)
	releaseCmdMachine, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		tracing.RecordError(span, err, "failed to get ip addresses")
		return fmt.Errorf("error creating a release_command machine: %w", err)
	}

	statuslogger.Logf(ctx, "Created release_command machine %s", md.colorize.Bold(releaseCmdMachine.ID))
	md.releaseCommandMachine = machine.NewMachineSet(md.flapsClient, md.io, []*fly.Machine{releaseCmdMachine}, true)

	lm := md.releaseCommandMachine.GetMachines()[0]
	if err := lm.WaitForState(ctx, fly.MachineStateStopped, md.waitTimeout, false); err != nil {
		err = suggestChangeWaitTimeout(err, "wait-timeout")
		return err
	}

	return nil
}

func (md *machineDeployment) launchInputForReleaseCommand(origMachineRaw *fly.Machine) *fly.LaunchMachineInput {
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

	return &fly.LaunchMachineInput{
		Config:     mConfig,
		Region:     origMachineRaw.Region,
		SkipLaunch: true,
	}
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

func (md *machineDeployment) waitForReleaseCommandToFinish(ctx context.Context, releaseCmdMachine machine.LeasableMachine) error {
	err := releaseCmdMachine.WaitForState(ctx, fly.MachineStateStarted, md.waitTimeout, false)
	if err != nil {
		var flapsErr *flaps.FlapsError
		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusNotFound {
			// The machine exited and was destroyed quickly.
			return nil
		}
		err = suggestChangeWaitTimeout(err, "wait-timeout")
		return fmt.Errorf("error waiting for release_command machine %s to start: %w", releaseCmdMachine.Machine().ID, err)
	}
	err = releaseCmdMachine.WaitForState(ctx, fly.MachineStateDestroyed, md.releaseCmdTimeout, true)
	if err != nil {
		err = suggestChangeWaitTimeout(err, "release-command-timeout")
		return fmt.Errorf("error waiting for release_command machine %s to finish running: %w", releaseCmdMachine.Machine().ID, err)
	}
	md.releaseCommandMachine.RemoveMachines(ctx, []machine.LeasableMachine{releaseCmdMachine})
	return nil
}
