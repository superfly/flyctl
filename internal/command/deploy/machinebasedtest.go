package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
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

type machineTestErr struct {
	testMachineLogs string
	exitCode        int
	machineID       string
}

func (e machineTestErr) Error() string {
	return fmt.Sprintf("Error test command machine %s exited with non-zero status of %d", e.machineID, e.exitCode)
}

func (e machineTestErr) Description() string {
	var desc string
	desc += fmt.Sprintf("Error: test command failed running on machine %s with exit code %d.\n", e.machineID, e.exitCode)
	desc += fmt.Sprintf("Check its logs: here's the last 100 lines below, or run 'fly logs -i %s':\n\n", e.machineID)
	desc += e.testMachineLogs
	return desc

}

func (md *machineDeployment) runTestMachines(ctx context.Context, machineToTest *fly.Machine, sl statuslogger.StatusLine) (err error) {
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

	if sl == nil {
		return fmt.Errorf("bug: status logger is nil")
	}

	processGroup := machineToTest.ProcessGroup()
	machineChecks := lo.FlatMap(md.appConfig.AllServices(), func(svc appconfig.Service, _ int) []*appconfig.ServiceMachineCheck {
		matchesProcessGroup := lo.Contains(svc.Processes, processGroup) || len(svc.Processes) == 0
		if matchesProcessGroup {
			return svc.MachineChecks
		} else {
			return nil
		}
	})
	machineChecks = append(machineChecks, md.appConfig.MachineChecks...)

	if len(machineChecks) == 0 {
		span.AddEvent("no machine checks")
		return nil
	}

	machines := lo.Map(machineChecks, func(machineCheck *appconfig.ServiceMachineCheck, _ int) createdTestMachine {
		var mach *fly.Machine
		var err error
		defer func() {
			if err != nil {
				sl.Failed(err)
			}
		}()

		mach, err = md.createTestMachine(ctx, machineCheck, machineToTest, sl)
		return createdTestMachine{mach, err}
	})

	if m, hasErr := lo.Find(machines, func(m createdTestMachine) bool {
		return m.err != nil
	}); hasErr {
		err := fmt.Errorf("error creating test machine: %w", m.err)
		tracing.RecordError(span, err, "failed to create test machine")
		return err
	}

	machineSet := machine.NewMachineSet(flaps, io, lo.FilterMap(machines, func(m createdTestMachine, _ int) (*fly.Machine, bool) {
		if m.err != nil {
			tracing.RecordError(span, m.err, "failed to create test machine")
			sl.LogStatus(statuslogger.StatusFailure, fmt.Sprintf("failed to create test machine: %s", m.err))
		}
		return m.mach, m.err == nil
	}), false)

	// FIXME: consolidate this wait stuff with deploy waits? Especially once we improve the output
	err = md.waitForTestMachinesToFinish(ctx, machineSet, sl)
	if err != nil {
		tracing.RecordError(span, err, "failed to wait for test cmd machine")
		return err
	}

	for _, testMachine := range machineSet.GetMachines() {
		md.waitForLogs(ctx, testMachine.Machine(), 10*time.Second)

		sl.Logf("Checking test command machine %s", md.colorize.Bold(testMachine.Machine().ID))
		lastExitEvent, err := testMachine.WaitForEventType(ctx, "exit", md.releaseCmdTimeout, true)
		if err != nil {
			return fmt.Errorf("error finding the test command machine %s exit event: %w", testMachine.Machine().ID, err)
		}
		exitCode, err := lastExitEvent.Request.GetExitCode()
		if err != nil {
			return fmt.Errorf("error get test command machine %s exit code: %w", testMachine.Machine().ID, err)
		}

		if exitCode != 0 {
			sl.LogStatus(statuslogger.StatusFailure, "test command failed")
			// Preemptive cleanup of the logger so that the logs have a clean place to write to

			testLogs, _, err := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, testMachine.Machine().ID)
			if err == nil {
				var logs string
				for _, l := range testLogs {
					logs += l.Message + "\n"
				}

				return machineTestErr{machineID: testMachine.Machine().ID, exitCode: exitCode, testMachineLogs: logs}
			}

			return fmt.Errorf("Error test command machine %s exited with non-zero status of %d", testMachine.Machine().ID, exitCode)
		}
		sl.LogfStatus(
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

	_, err := backoff.Retry(ctx, func() ([]fly.LogEntry, error) {
		logs, _, err := md.apiClient.GetAppLogs(ctx, md.app.Name, "", md.appConfig.PrimaryRegion, mach.ID)
		if err != nil {
			return nil, err
		}
		if len(logs) == 0 {
			return nil, fmt.Errorf(ErrNoLogsFound)
		}

		return logs, nil
	}, backoff.WithBackOff(b), backoff.WithMaxElapsedTime(timeout))
	return err
}

func (md *machineDeployment) createTestMachine(ctx context.Context, svc *appconfig.ServiceMachineCheck, machineToTest *fly.Machine, sl statuslogger.StatusLine) (*fly.Machine, error) {
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

	sl.Logf("Created test machine %s", md.colorize.Bold(testMachine.ID))
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

	// The canary function works just as well for test machines
	mConfig.Guest = md.inferCanaryGuest(mConfig.ProcessGroup())
	md.setMachineReleaseData(mConfig)

	if hdid := md.appConfig.HostDedicationID; hdid != "" {
		mConfig.Guest.HostDedicationID = hdid
	}

	return &fly.LaunchMachineInput{
		Config: mConfig,
		Region: origMachineRaw.Region,
	}, nil
}

func (md *machineDeployment) waitForTestMachinesToFinish(ctx context.Context, testMachines machine.MachineSet, sl statuslogger.StatusLine) error {
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
		sl.Logf("Test Machine %s: %s", colorize.Bold(mach.ID), mach.State)
	}

	return nil
}
