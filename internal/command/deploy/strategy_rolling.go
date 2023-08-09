package deploy

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/machine"
)

const (
	rollingStrategyMaxConcurrentGroups = 10
)

func (md *machineDeployment) updateUsingRollingStrategy(ctx context.Context, updateEntries []*machineUpdateEntry) error {
	entriesByGroup := lo.GroupBy(updateEntries, func(e *machineUpdateEntry) string {
		return e.launchInput.Config.ProcessGroup()
	})

	groupsPool := pool.New().WithErrors().WithMaxGoroutines(rollingStrategyMaxConcurrentGroups).WithContext(ctx)
	for _, entries := range entriesByGroup {
		entries := entries
		groupsPool.Go(func(ctx context.Context) error {
			return md.updateMachineEntries(ctx, entries)
		})
	}
	return groupsPool.Wait()
}

func (md *machineDeployment) updateMachineEntries(ctx context.Context, entries []*machineUpdateEntry) error {
	var poolSize int
	switch mu := md.maxUnavailable; {
	case mu >= 1:
		poolSize = int(mu)
	case mu > 0:
		poolSize = int(math.Ceil(float64(len(entries)) * mu))
	default:
		return fmt.Errorf("Invalid --max-unavailable value: %v", mu)
	}

	updatePool := pool.New().WithErrors().WithMaxGoroutines(poolSize).WithContext(ctx)

	for i, e := range entries {
		e := e
		indexStr := formatIndex(i, len(entries))
		updatePool.Go(func(ctx context.Context) error {
			if err := md.updateMachine(ctx, e, indexStr); err != nil {
				if md.strategy == "immediate" {
					fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
					return nil
				}
				return err
			}

			if err := md.waitForMachine(ctx, e.leasableMachine, true, indexStr); err != nil {
				return err
			}
			md.logClearLinesAbove(1)
			return nil
		})
	}

	if err := updatePool.Wait(); err != nil {
		return err
	}

	fmt.Fprintf(md.io.ErrOut, "  Finished deploying\n")
	return nil
}

func (md *machineDeployment) updateMachine(ctx context.Context, e *machineUpdateEntry, indexStr string) error {
	if e.launchInput.RequiresReplacement {
		return md.updateMachineByReplace(ctx, e, indexStr)
	}

	if err := md.updateMachineInPlace(ctx, e, indexStr); err != nil {
		switch {
		case len(e.leasableMachine.Machine().Config.Mounts) > 0:
			// Replacing a machine with a volume will cause the placement logic to pick wthe same host
			// dismissing the value of replacing it in case of lack of host capacity
			return err
		case strings.Contains(err.Error(), "could not reserve resource for machine"):
			return md.updateMachineByReplace(ctx, e, indexStr)
		default:
			return err
		}
	}
	return nil
}

func (md *machineDeployment) updateMachineByReplace(ctx context.Context, e *machineUpdateEntry, indexStr string) error {
	lm := e.leasableMachine
	// If machine requires replacement, destroy old machine and launch a new one
	// This can be the case for machines that changes its volumes.
	fmt.Fprintf(md.io.ErrOut, "  %s Replacing %s by new machine\n", indexStr, md.colorize.Bold(lm.FormattedMachineId()))
	if err := lm.Destroy(ctx, true); err != nil {
		return err
	}

	// Acquire a lease on the new machine to ensure external factors can't stop or update it
	// while we wait for its state and/or health checks
	e.launchInput.LeaseTTL = int(md.waitTimeout.Seconds())

	newMachineRaw, err := md.flapsClient.Launch(ctx, *e.launchInput)
	if err != nil {
		return err
	}

	lm = machine.NewLeasableMachine(md.flapsClient, md.io, newMachineRaw)
	fmt.Fprintf(md.io.ErrOut, "  %s Created machine %s\n", indexStr, md.colorize.Bold(lm.FormattedMachineId()))
	defer lm.ReleaseLease(ctx)
	e.leasableMachine = lm
	return nil
}

func (md *machineDeployment) updateMachineInPlace(ctx context.Context, e *machineUpdateEntry, indexStr string) error {
	lm := e.leasableMachine
	fmt.Fprintf(md.io.ErrOut, "  %s Updating %s\n", indexStr, md.colorize.Bold(lm.FormattedMachineId()))
	if err := lm.Update(ctx, *e.launchInput); err != nil {
		if md.strategy != "immediate" {
			return err
		}
		fmt.Fprintf(md.io.ErrOut, "Continuing after error: %s\n", err)
	}
	return nil
}

func (md *machineDeployment) waitForMachine(ctx context.Context, lm machine.LeasableMachine, inBatch bool, indexStr string) error {
	if md.strategy == "immediate" {
		return nil
	}

	// Don't wait for Standby machines, they are updated but not started
	if len(lm.Machine().Config.Standbys) > 0 {
		md.logClearLinesAbove(1)
		fmt.Fprintf(md.io.ErrOut, "  %s Machine %s update finished: %s\n",
			indexStr,
			md.colorize.Bold(lm.FormattedMachineId()),
			md.colorize.Green("success"),
		)
		return nil
	}

	if !md.skipHealthChecks {
		if err := lm.WaitForState(ctx, api.MachineStateStarted, md.waitTimeout, indexStr, false); err != nil {
			err = suggestChangeWaitTimeout(err, "wait-timeout")
			return err
		}
	}

	if err := md.doSmokeChecks(ctx, lm, indexStr); err != nil {
		return err
	}

	if !md.skipHealthChecks {
		if err := lm.WaitForHealthchecksToPass(ctx, md.waitTimeout, indexStr); err != nil {
			md.warnAboutIncorrectListenAddress(ctx, lm)
			err = suggestChangeWaitTimeout(err, "wait-timeout")
			return err
		}
		// FIXME: combine this wait with the wait for start as one update line (or two per in noninteractive case)
		md.logClearLinesAbove(1)
		fmt.Fprintf(md.io.ErrOut, "  %s Machine %s update finished: %s\n",
			indexStr,
			md.colorize.Bold(lm.FormattedMachineId()),
			md.colorize.Green("success"),
		)
		if inBatch {
			fmt.Fprint(md.io.ErrOut, "\n")
		}
	}

	md.warnAboutIncorrectListenAddress(ctx, lm)
	return nil
}
