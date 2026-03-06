package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func newWait() *cobra.Command {
	const (
		short = "Wait for a machine to reach a state"
		long  = short + "\n"

		usage = "wait [id]"
	)

	cmd := command.New(usage, short, long, runMachineWait,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.String{
			Name:        "state",
			Description: "Machine state to wait for",
			Default:     "settled",
		},
		flag.Duration{
			Name:        "wait-timeout",
			Shorthand:   "w",
			Description: "Time duration to wait for the machine to reach the requested state.",
			Default:     5 * time.Minute,
		},
	)

	return cmd
}

func runMachineWait(ctx context.Context) error {
	var (
		io           = iostreams.FromContext(ctx)
		desiredState = flag.GetString(ctx, "state")
		waitTimeout  = flag.GetDuration(ctx, "wait-timeout")
	)

	if desiredState == "" {
		return fmt.Errorf("state cannot be empty")
	}

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	machine, ctx, err := selectOneMachine(ctx, "", machineID, haveMachineID)
	if err != nil {
		return err
	}

	appName := appconfig.NameFromContext(ctx)
	client := flapsutil.ClientFromContext(ctx)

	fmt.Fprintf(io.Out, "Waiting up to %s for machine %s to reach %q...\n", waitTimeout, machine.ID, desiredState)

	startedWaitAt := time.Now()
	const maxAttempts = 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		remainingTimeout := waitTimeout
		if waitTimeout > 0 {
			remainingTimeout = waitTimeout - time.Since(startedWaitAt)
			if remainingTimeout <= 0 {
				return fmt.Errorf("machine %s did not reach %q within %s", machine.ID, desiredState, waitTimeout)
			}
		}

		err = client.Wait(ctx, appName, machine, desiredState, remainingTimeout)
		if err == nil {
			break
		}

		if attempt == maxAttempts || !isRetryableWaitError(err) {
			return fmt.Errorf("machine %s did not reach %q within %s: %w", machine.ID, desiredState, waitTimeout, err)
		}

		fmt.Fprintf(io.Out, "Retrying wait for machine %s due to transient error: %v\n", machine.ID, err)

		machine, err = client.Get(ctx, appName, machine.ID)
		if err != nil {
			return fmt.Errorf("machine %s could not be refetched before retrying wait: %w", machine.ID, err)
		}

		retryDelay := retryDelayForAttempt(attempt)
		if waitTimeout > 0 {
			remainingTimeout = waitTimeout - time.Since(startedWaitAt)
			if remainingTimeout <= 0 {
				return fmt.Errorf("machine %s did not reach %q within %s", machine.ID, desiredState, waitTimeout)
			}
			if retryDelay > remainingTimeout {
				retryDelay = remainingTimeout
			}
		}

		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if desiredState == "settled" {
		machine, err = client.Get(ctx, appName, machine.ID)
		if err != nil {
			return fmt.Errorf("machine %s reached settled state but could not fetch final state: %w", machine.ID, err)
		}
		desiredState = machine.State
	}

	fmt.Fprintf(io.Out, "Machine %s reached state %q\n", machine.ID, desiredState)

	return nil
}

func isRetryableWaitError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "currently replaced") {
		return true
	}

	var flapsErr *flaps.FlapsError
	if errors.As(err, &flapsErr) {
		if flapsErr.ResponseStatusCode == http.StatusTooManyRequests || (flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600) {
			return true
		}
	}

	transientSubstrings := []string{
		"connection reset by peer",
		"connection refused",
		"network is unreachable",
		"temporary failure in name resolution",
		"i/o timeout",
		"timeout",
		"eof",
	}

	for _, s := range transientSubstrings {
		if strings.Contains(message, s) {
			return true
		}
	}

	return false
}

func retryDelayForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		return 500 * time.Millisecond
	}

	delay := 500 * time.Millisecond
	for i := 1; i < attempt; i++ {
		delay *= 2
	}

	return delay
}
