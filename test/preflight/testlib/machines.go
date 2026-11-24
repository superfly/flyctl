//go:build integration

package testlib

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
)

// WaitForMachines polls only the list command until all caller assertions pass.
// Use assert (not require) in check, and guard indexing with assert.Len/NotEmpty.
// The command deadline is inside the polling deadline: no tick survives the wait.
func (f *FlyctlTestEnv) WaitForMachines(app string, check func(assert.TestingT, []*fly.Machine)) []*fly.Machine {
	f.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	machines, err := f.waitForMachines(ctx, app, check, time.Second)
	if err != nil {
		f.Fatalf("%v", err)
	}

	return machines
}

func (f *FlyctlTestEnv) WaitForMachineCount(app string, count int) []*fly.Machine {
	f.Helper()

	return f.WaitForMachines(app, func(t assert.TestingT, machines []*fly.Machine) {
		assert.Len(t, machines, count)
	})
}

type machineAssertions struct{ errors []string }

func (a *machineAssertions) Errorf(format string, args ...any) {
	a.errors = append(a.errors, fmt.Sprintf(format, args...))
}

func (f *FlyctlTestEnv) waitForMachines(ctx context.Context, app string, check func(assert.TestingT, []*fly.Machine), interval time.Duration) ([]*fly.Machine, error) {
	view := *f
	view.retrying = true
	last := "no list response"
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res := view.FlyContextAndConfig(attemptCtx, FlyCmdConfig{NoAssertSuccessfulExit: true, NoAssertDeadline: true}, "machines list --app %s --json", app)
		cancel()
		if ctx.Err() != nil {
			// A killed final read must not erase the last completed assertion.
			break
		}
		var machines []*fly.Machine
		switch {
		case res.ExitCode() != 0:
			last = fmt.Sprintf("list exited %d: %s", res.ExitCode(), res.StdErrString())
		default:
			if err := json.Unmarshal(res.StdOut().Bytes(), &machines); err != nil {
				return nil, fmt.Errorf("invalid machine list JSON: %w; output: %s", err, res.StdOutString())
			}
			a := new(machineAssertions)
			check(a, machines)
			if len(a.errors) == 0 && ctx.Err() == nil {
				return machines, nil
			}
			last = strings.Join(a.errors, "\n")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}

	return nil, fmt.Errorf("machine list for %s never matched: %w; last result: %s", app, ctx.Err(), last)
}
