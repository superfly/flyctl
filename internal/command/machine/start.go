package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func newStart() *cobra.Command {
	const (
		short = "Start one or more Fly machines"
		long  = short + "\n"

		usage = "start [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineStart,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.Bool{
			Name:        "wait",
			Description: "Wait for the Machines to start and pass configured health checks.",
		},
	)

	return cmd
}

func runMachineStart(ctx context.Context) (err error) {
	args := flag.Args(ctx)

	machines, ctx, err := selectManyMachines(ctx, args)
	if err != nil {
		return err
	}

	// appName is added to context by selectManyMachines
	appName := appconfig.NameFromContext(ctx)

	// Finish the initial batch under its normal leases. A replacement can
	// take longer than a lease, so release the batch before waiting for any.
	var replacements []*fly.Machine
	err = func() error {
		leased, release, err := mach.AcquireLeases(ctx, appName, machines)
		defer release()
		if err != nil {
			return err
		}
		for _, machine := range leased {
			err := startSelected(ctx, appName, machine, false)
			if machine.Version != "" && refusedReplacementStart(err) {
				replacements = append(replacements, machine)
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}
	for _, machine := range replacements {
		if err := startReplacement(ctx, appName, machine); err != nil {
			return err
		}
	}
	return nil
}

func startSelected(ctx context.Context, appName string, machine *fly.Machine, confirmedStart bool) error {
	var err error
	if !confirmedStart {
		if err = tracing.WorkflowPhase(ctx, "request_start", func(ctx context.Context) error {
			err := Start(ctx, appName, machine)
			created := startRefusedWhileCreated(err)
			if machine.Version == "" || (!created && !startAlreadyInProgress(err)) {
				return err
			}

			states := []string{fly.MachineStateStarted}
			if created {
				states = append(states, fly.MachineStateStopped)
			}
			const timeout = time.Minute
			waitCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			err = flapsutil.ClientFromContext(ctx).Wait(waitCtx, appName, machine.ID,
				flaps.WithWaitVersion(machine.Version), flaps.WithWaitStates(states...), flaps.WithWaitTimeout(timeout))
			if err != nil {
				return fmt.Errorf("waiting for machine %s to finish starting: %w", machine.ID, err)
			}

			if created {
				confirmedStart, err = startAfterCreation(waitCtx, appName, machine)
				return err
			}
			confirmedStart = true

			return nil
		}); err != nil {
			return err
		}
	}
	if flag.GetBool(ctx, "wait") {
		if !confirmedStart {
			if err = tracing.WorkflowPhase(ctx, "wait_for_start", func(ctx context.Context) error {
				return mach.WaitForStartOrStop(ctx, appName, machine, "start", 5*time.Minute)
			}); err != nil {
				return err
			}
		}
		if len(machine.Checks) > 0 {
			if err = tracing.WorkflowPhase(ctx, "wait_for_health_checks", func(ctx context.Context) error {
				return watch.MachinesChecks(ctx, appName, []*fly.Machine{machine})
			}); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(iostreams.FromContext(ctx).Out, "%s has been started\n", machine.ID)
	return nil
}

func refusedReplacementStart(err error) bool {
	var apiErr *flaps.FlapsError
	if !errors.As(err, &apiErr) || apiErr.ResponseStatusCode != http.StatusPreconditionFailed {
		return false
	}
	const refusal = "machine getting replaced, refusing to start"
	msg := strings.TrimPrefix(apiErr.Error(), "failed_precondition: ")
	return msg == refusal || msg == refusal+". Retry the start request after the replacement finishes"
}

func startReplacement(ctx context.Context, appName string, original *fly.Machine) error {
	client := flapsutil.ClientFromContext(ctx)
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		current, err := client.Get(waitCtx, appName, original.ID)
		if err != nil {
			return err
		}
		if current.Version != "" && current.Version != original.Version {
			switch current.State {
			case fly.MachineStateStarted:
				return startSelected(ctx, appName, current, true)
			case fly.MachineStateStopped:
				// AcquireLease re-fetches the Machine if the lease version
				// differs. Only retry the successor we observed stopped.
				leased, release, err := mach.AcquireLease(waitCtx, appName, current)
				defer release()
				if err != nil {
					return err
				}
				if leased != current || leased.LeaseNonce == "" {
					return fmt.Errorf("machine %s changed while acquiring its replacement lease", current.ID)
				}
				return startSelected(ctx, appName, current, false)
			}
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("waiting for machine %s replacement: %w", original.ID, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

// Wait does not return the matched state to flyctl.
func startAfterCreation(ctx context.Context, appName string, machine *fly.Machine) (bool, error) {
	client := flapsutil.ClientFromContext(ctx)
	current, err := client.Get(ctx, appName, machine.ID)
	if err != nil {
		return false, err
	}
	if current.Version != machine.Version {
		return false, fmt.Errorf("machine %s changed version while waiting for creation", machine.ID)
	}
	switch current.State {
	case fly.MachineStateStarted:
		return true, nil
	case fly.MachineStateStopped:
		// The start endpoint takes a lease nonce but no version. The lease
		// acquired before the first request may have expired while waiting.
		lease, err := client.FindLease(ctx, appName, machine.ID)
		if err != nil {
			return false, err
		}
		if lease.Data == nil || machine.LeaseNonce == "" || lease.Data.Nonce != machine.LeaseNonce || lease.Data.Version != machine.Version {
			return false, fmt.Errorf("machine %s no longer holds the original lease", machine.ID)
		}
		startCtx, cancel := context.WithDeadline(ctx, time.Unix(lease.Data.ExpiresAt, 0))
		defer cancel()
		if err := startCtx.Err(); err != nil {
			return false, err
		}

		return false, Start(startCtx, appName, machine)
	default:
		return false, fmt.Errorf("machine %s entered state %q after creation", machine.ID, current.State)
	}
}

func startAlreadyInProgress(err error) bool {
	var apiErr *flaps.FlapsError
	return errors.As(err, &apiErr) && apiErr.ResponseStatusCode == http.StatusConflict &&
		strings.TrimPrefix(apiErr.Error(), "aborted: ") == "machine still attempting to start"
}

func startRefusedWhileCreated(err error) bool {
	var apiErr *flaps.FlapsError
	return errors.As(err, &apiErr) && apiErr.ResponseStatusCode == http.StatusPreconditionFailed &&
		strings.TrimPrefix(apiErr.Error(), "failed_precondition: ") == "unable to start machine from current state: 'created'"
}

func Start(ctx context.Context, appName string, machine *fly.Machine) (err error) {
	res, err := flapsutil.ClientFromContext(ctx).Start(ctx, appName, machine.ID, machine.LeaseNonce)
	if err != nil {
		// TODO(dov): just do the clone
		switch {
		case strings.Contains(err.Error(), " for machine"):
			return fmt.Errorf("could not start machine due to lack of capacity. Try 'flyctl machine clone %s -a %s'", machine.ID, appconfig.NameFromContext(ctx))
		default:
			if err := rewriteMachineNotFoundErrors(ctx, err, machine.ID); err != nil {
				return err
			}

			return fmt.Errorf("could not start machine %s: %w", machine.ID, err)
		}
	}

	if res.Status == "error" {
		return fmt.Errorf("machine could not be started: %s", res.Message)
	}

	return
}
