package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
)

// FlyctlLaunchIDMetadataKey is a per-launch idempotency tag written by flyctl
// into a machine's config metadata before calling flaps.Launch. It exists so
// LaunchWithIdempotency can detect a "silent success" (a Launch that hit an
// ambiguous 408/5xx but committed the machine anyway) and reuse the
// committed machine instead of creating a duplicate on retry.
//
// The key is intentionally namespaced under "fly_flyctl_" so it can coexist
// with any user metadata.
const FlyctlLaunchIDMetadataKey = "fly_flyctl_launch_id"

// LaunchIdempotencyOpts controls the retry behaviour of LaunchWithIdempotency.
type LaunchIdempotencyOpts struct {
	// RetryAttempts is the maximum number of Launch attempts (including the
	// first). Must be >= 1; a value of 0 is treated as 1 (no retries).
	RetryAttempts uint

	// RetryDelay is the initial back-off between attempts. Doubled between
	// attempts up to a hard cap of 5s.
	RetryDelay time.Duration

	// LookupDelay is the pause between a failed Launch attempt and the
	// idempotency lookup that follows it. It gives flaps' backing store a
	// moment to reflect a machine that a silent-success Launch just
	// committed, which reduces (but does not eliminate) the race where a
	// retry could otherwise create a duplicate.
	LookupDelay time.Duration

	// OnRetry, when set, is invoked before each retry attempt (i.e. after
	// the *first* failure and any subsequent failures). `attempt` is the
	// 1-indexed attempt number about to be made.
	OnRetry func(attempt, total uint, err error)

	// OnSilentSuccess, when set, is invoked when the idempotency lookup
	// finds a machine that a prior failed-looking attempt actually
	// committed. Typically used for logging.
	OnSilentSuccess func(m *fly.Machine)
}

func (o *LaunchIdempotencyOpts) applyDefaults() {
	if o.RetryAttempts == 0 {
		o.RetryAttempts = 3
	}
	if o.RetryDelay == 0 {
		o.RetryDelay = 500 * time.Millisecond
	}
	// LookupDelay is allowed to be zero for tests \u2014 don't fill it in.
}

// LaunchWithIdempotency wraps flaps.Launch with client-side pseudo-idempotency.
//
// flaps' create-machine endpoint doesn't accept an Idempotency-Key header, so
// we simulate one: this helper stamps a unique ULID into the launch input's
// metadata under FlyctlLaunchIDMetadataKey (unless the caller already set
// one), calls Launch, and on transient failure lists machines for the app
// looking for one carrying that ID before retrying. If it finds one, the
// "failure" is treated as a silent success and the committed machine is
// returned \u2014 no duplicate is created.
//
// The race we can't fully close is when flaps commits the machine but the
// lookup runs before that write propagates to the read side flaps' List
// handler queries. LookupDelay gives it a brief window to catch up; in the
// unlikely event a duplicate does slip through it will be identifiable by
// the shared idempotency tag and can be reconciled by the caller.
//
// Non-transient failures (validation errors, 4xx other than 408/429) fail
// on the first attempt without retry.
func LaunchWithIdempotency(
	ctx context.Context,
	flapsClient flapsutil.FlapsClient,
	appName string,
	input *fly.LaunchMachineInput,
	opts LaunchIdempotencyOpts,
) (*fly.Machine, error) {
	opts.applyDefaults()

	if input.Config == nil {
		return nil, fmt.Errorf("LaunchWithIdempotency: input.Config must not be nil")
	}
	if input.Config.Metadata == nil {
		input.Config.Metadata = map[string]string{}
	}

	launchID := input.Config.Metadata[FlyctlLaunchIDMetadataKey]
	if launchID == "" {
		launchID = ulid.Make().String()
		input.Config.Metadata[FlyctlLaunchIDMetadataKey] = launchID
	}

	var lastErr error
	delay := opts.RetryDelay
	for attempt := uint(0); attempt < opts.RetryAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		created, err := flapsClient.Launch(ctx, appName, *input)
		if err == nil {
			return created, nil
		}
		lastErr = err

		if !flapsutil.IsTransientFlapsError(err) {
			return nil, err
		}

		// Might have been a silent success. Give flaps' backing store a
		// moment to catch up, then look for a machine with our launch ID.
		if opts.LookupDelay > 0 {
			select {
			case <-time.After(opts.LookupDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		existing, lookupErr := findMachineByLaunchID(ctx, flapsClient, appName, launchID)
		if lookupErr == nil && existing != nil {
			if opts.OnSilentSuccess != nil {
				opts.OnSilentSuccess(existing)
			}

			return existing, nil
		}

		if attempt+1 < opts.RetryAttempts {
			if opts.OnRetry != nil {
				opts.OnRetry(attempt+2, opts.RetryAttempts, err)
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if delay < 5*time.Second {
				delay *= 2
			}
		}
	}

	return nil, lastErr
}

// findMachineByLaunchID scans the app's machines and returns the one whose
// metadata carries the given launch ID, if any. It's a client-side filter
// because flaps' List does not (yet) expose a metadata query in the fly-go
// client interface.
func findMachineByLaunchID(ctx context.Context, flapsClient flapsutil.FlapsClient, appName, launchID string) (*fly.Machine, error) {
	machines, err := flapsClient.List(ctx, appName, "")
	if err != nil {
		return nil, err
	}

	for _, m := range machines {
		if m == nil || m.Config == nil {
			continue
		}
		if m.Config.Metadata[FlyctlLaunchIDMetadataKey] == launchID {
			return m, nil
		}
	}

	return nil, nil
}
