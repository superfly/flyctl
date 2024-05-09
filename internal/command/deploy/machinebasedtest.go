package deploy

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
)

type createdTestMachine struct {
	mach *fly.Machine
	err  error
}

func (md *machineDeployment) runTestMachines(ctx context.Context, machineToTest *fly.Machine) (err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "run_test_machine")
	var (
		flaps = md.flapsClient
		io    = md.io
	)
	defer func() {
		if err != nil {
			tracing.RecordError(span, err, "failed to run test machine")
		}
		span.End()
	}()

	processGroup := machineToTest.ProcessGroup()
	machineChecks := lo.FlatMap(md.appConfig.AllServices(), func(svc appconfig.Service, _ int) []*appconfig.ServiceMachineCheck {
		matchesProcessGroup := lo.Contains(svc.Processes, processGroup)
		if matchesProcessGroup {
			return nil
		} else {
			return svc.MachineChecks
		}
	})

	if len(machineChecks) == 0 {
		span.AddEvent("no machine checks")
		return nil
	}

	machines := lo.Map(machineChecks, func(machineCheck *appconfig.ServiceMachineCheck, _ int) createdTestMachine {
		var mach *fly.Machine
		var err error
		defer func() {
			if err != nil {
				statuslogger.Failed(ctx, err)
			}
		}()

		mach, err = md.createTestMachine(ctx, machineCheck, machineToTest)
		if err != nil {
			err = fmt.Errorf("error creating test machine %q: %w", machineCheck.Command, err)
		}

		return createdTestMachine{mach, err}
	})

	if m, hasErr := lo.Find(machines, func(m createdTestMachine) bool {
		return m.err != nil
	}); hasErr {
		err := fmt.Errorf("error creating test machine %s: %w", m.mach.ID, m.err)
		tracing.RecordError(span, err, "failed to create test machine")
		return err
	}

	machineSet := machine.NewMachineSet(flaps, io, lo.FilterMap(machines, func(m createdTestMachine, _ int) (*fly.Machine, bool) {
		if m.err != nil {
			tracing.RecordError(span, m.err, "failed to create test machine")
			statuslogger.LogStatus(ctx, statuslogger.StatusFailure, fmt.Sprintf("failed to create test machine: %s", m.err))
		}
		return m.mach, m.err == nil
	}))

	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the output
	err = md.waitForTestMachinesToFinish(ctx, machineSet)
	if err != nil {
		tracing.RecordError(span, err, "failed to wait for test cmd machine")
		return err
	}

	for _, testMachine := range machineSet.GetMachines() {
		md.waitForLogs(ctx, testMachine.Machine(), 10*time.Second)

		statuslogger.Logf(ctx, "Checking test command machine %s", md.colorize.Bold(testMachine.Machine().ID))
		lastExitEvent, err := testMachine.WaitForEventType(ctx, "exit", md.releaseCmdTimeout, true)
		if err != nil {
			return fmt.Errorf("error finding the test command machine %s exit event: %w", testMachine.Machine().ID, err)
		}
		exitCode, err := lastExitEvent.Request.GetExitCode()
		if err != nil {
			return fmt.Errorf("error get test command machine %s exit code: %w", testMachine.Machine().ID, err)
		}

		if exitCode != 0 {
			statuslogger.LogStatus(ctx, statuslogger.StatusFailure, "test command failed")
			// Preemptive cleanup of the logger so that the logs have a clean place to write to

			fmt.Fprintf(md.io.ErrOut, "Error: test command failed running on machine %s with exit code %s.\n",
				md.colorize.Bold(testMachine.Machine().ID), md.colorize.Red(strconv.Itoa(exitCode)))
			fmt.Fprintf(md.io.ErrOut, "Check its logs: here's the last 100 lines below, or run 'fly logs -i %s':\n",
				testMachine.Machine().ID)
			testLogs, _, err := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, testMachine.Machine().ID)
			if fly.IsNotAuthenticatedError(err) {
				fmt.Fprintf(md.io.ErrOut, "Warn: not authorized to retrieve app logs (this can happen when using deploy tokens), so we can't show you what failed. Use `fly logs -i %s` or open the monitoring dashboard to see them: https://fly.io/apps/%s/monitoring?region=&instance=%s\n", testMachine.Machine().ID, md.appConfig.AppName, testMachine.Machine().ID)
			} else {
				if err != nil {
					return fmt.Errorf("Error getting test command logs: %w", err)
				}
				for _, l := range testLogs {
					fmt.Fprintf(md.io.ErrOut, "  %s\n", l.Message)
				}
			}
			return fmt.Errorf("Error test command machine %s exited with non-zero status of %d", testMachine.Machine().ID, exitCode)
		}
		statuslogger.LogfStatus(ctx,
			statuslogger.StatusSuccess,
			"Test machine %s completed successfully",
			md.colorize.Bold(testMachine.Machine().ID),
		)
	}

	return nil
}

const ErrNoLogsFound = "no logs found"

func (md *machineDeployment) waitForLogs(ctx context.Context, mach *fly.Machine, timeout time.Duration) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 1 * time.Second
	b.MaxInterval = 10 * time.Second
	b.MaxElapsedTime = timeout

	return backoff.Retry(func() error {
		logs, _, err := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, mach.ID)
		if err != nil {
			return err
		}
		if len(logs) == 0 {
			return fmt.Errorf(ErrNoLogsFound)
		}

		return nil
	}, backoff.WithContext(b, ctx))
}

func (md *machineDeployment) createTestMachine(ctx context.Context, svc *appconfig.ServiceMachineCheck, machineToTest *fly.Machine) (*fly.Machine, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "create_test_machine")
	defer span.End()

	launchInput, err := md.launchInputForTestMachine(svc, machineToTest)
	if err != nil {
		return nil, err
	}
	testMachine, err := md.flapsClient.Launch(ctx, *launchInput)
	if err != nil {
		tracing.RecordError(span, err, "failed to create test machines")
		return nil, fmt.Errorf("error creating a test machine: %w", err)
	}

	statuslogger.Logf(ctx, "Created test machine %s", md.colorize.Bold(testMachine.ID))
	return testMachine, nil
}

func (md *machineDeployment) launchInputForTestMachine(svc *appconfig.ServiceMachineCheck, origMachineRaw *fly.Machine) (*fly.LaunchMachineInput, error) {
	if origMachineRaw == nil {
		origMachineRaw = &fly.Machine{
			Region: md.appConfig.PrimaryRegion,
		}
	}

	mConfig, err := md.appConfig.ToTestMachineConfig(svc, origMachineRaw)
	if err != nil {
		return nil, err
	}
	mConfig.Guest = md.inferTestMachineGuest()
	md.setMachineReleaseData(mConfig)

	if hdid := md.appConfig.HostDedicationID; hdid != "" {
		mConfig.Guest.HostDedicationID = hdid
	}

	return &fly.LaunchMachineInput{
		Config: mConfig,
		Region: origMachineRaw.Region,
	}, nil
}

func (md *machineDeployment) inferTestMachineGuest() *fly.MachineGuest {
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

func (md *machineDeployment) waitForTestMachinesToFinish(ctx context.Context, testMachines machine.MachineSet) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// I wish waitForMachines didn't 404, but I get why
	badMachineIDs, err := testMachines.WaitForMachineSetState(ctx, fly.MachineStateStarted, md.waitTimeout, false, true)
	if err != nil {
		err = suggestChangeWaitTimeout(err, "wait-timeout")
		for _, mach := range badMachineIDs {
			err = fmt.Errorf("%w\n%s", err, mach)
		}
		return fmt.Errorf("error waiting for test command machines to start: %w", err)
	}

	badMachineIDs, err = testMachines.WaitForMachineSetState(ctx, fly.MachineStateDestroyed, md.waitTimeout, false, false)
	if err != nil {
		err = suggestChangeWaitTimeout(err, "wait-timeout")
		for _, mach := range badMachineIDs {
			err = fmt.Errorf("%w\n%s", err, mach)
		}
		return fmt.Errorf("error waiting for test command machines to finish running: %w", err)
	}

	machs := lo.FilterMap(testMachines.GetMachines(), func(lm machine.LeasableMachine, _ int) (*fly.Machine, bool) {
		mach := lm.Machine()
		m, err := md.flapsClient.Get(ctx, mach.ID)
		return m, err == nil
	})
	for _, mach := range machs {
		statuslogger.Logf(ctx, "Test Machine %s: %s", colorize.Bold(mach.ID), mach.State)
	}

	return nil
}
