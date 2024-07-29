package watch

import (
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/morikuni/aec"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func MachinesChecks(ctx context.Context, machines []*fly.Machine) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	checksTotal := lo.SumBy(machines, func(m *fly.Machine) int { return len(m.Checks) })
	if checksTotal == 0 {
		fmt.Fprintln(io.Out, "No health checks found")
		return nil
	}

	machineIDs := lo.Map(machines, func(m *fly.Machine, _ int) string { return m.ID })
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	iteration := 0

	fn := func() error {
		checked, err := retryGetMachines(ctx, machineIDs...)
		if err != nil {
			return retry.Unrecoverable(err)
		}

		iteration++
		if io.IsInteractive() && iteration > 1 {
			builder := aec.EmptyBuilder
			str := builder.Up(uint(len(checked))).EraseLine(aec.EraseModes.All).ANSI
			fmt.Fprint(io.ErrOut, str.String())
		}

		checksPassed := 0
		for _, machine := range checked {
			if len(machine.Checks) == 0 {
				continue
			}
			checkStatus := machine.AllHealthChecks()
			checksPassed += checkStatus.Passing
			// Waiting for xxxxxxxx to become healthy (started, 3/3)
			fmt.Fprintf(io.ErrOut, "  Waiting for %s to become healthy (%s, %s)\n",
				colorize.Bold(machine.ID),
				colorize.Green(machine.State),
				colorize.Green(fmt.Sprintf("%d/%d", checkStatus.Passing, checkStatus.Total)),
			)
		}

		// if all checks are passing, we're done
		if checksPassed != checksTotal {
			return fmt.Errorf("Waiting for %d non-passing checks", checksTotal-checksPassed)
		}
		return nil
	}

	return retry.Do(fn, retry.Delay(time.Second), retry.DelayType(retry.FixedDelay), retry.Attempts(0), retry.Context(ctx))
}

// retryGetMachines calls flaps with exponential backoff 10s max interval and up to 6 times
func retryGetMachines(ctx context.Context, machineIDs ...string) (result []*fly.Machine, err error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	err = retry.Do(
		func() (err2 error) {
			result, err2 = flapsClient.GetMany(ctx, machineIDs)
			return err2
		},
		retry.Attempts(6), retry.MaxDelay(10*time.Second), retry.Context(ctx),
	)
	return
}
