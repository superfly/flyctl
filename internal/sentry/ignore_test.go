package sentry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	getsentry "github.com/getsentry/sentry-go"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flyerr"
	"golang.org/x/crypto/ssh"
)

var stepLabel = flyerr.StepLabel(errors.New("could not get all green machines to be healthy"))

func TestIgnore(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"cancelled", fmt.Errorf("deploy: %w", context.Canceled), true},
		{"ctrl-c through context.Cause", errors.New(`Post "https://api.fly.io/api/v1/builds": interrupt signal received`), true},
		{"sigterm through context.Cause", errors.New(`Post "https://api.fly.io/api/v1/releases": terminated signal received`), true},
		{"remote exit status", fmt.Errorf("ssh shell: %w", &ssh.ExitError{}), true},
		{"machine limit", errors.New("Your organization has reached its machine limit. Please contact billing@fly.io"), true},
		{"overdue invoices", errors.New("failed to create release (status 403): Your account has overdue invoices."), true},
		{"not authorized is still reported", errors.New("failed to run create_wg_peers: Not authorized to manage this organization::feature."), false},
		{"marked expected", fmt.Errorf("smoke checks for d89 failed: %w", flyerr.Expected(errors.New("the app appears to be crashing"))), true},
		{"expected text without the mark is reported", errors.New("the app appears to be crashing"), false},
		{"missing exit status is reported", fmt.Errorf("ssh shell: %w", &ssh.ExitMissingError{}), false},
		{"hangup signal", errors.New("session forcibly closed; the remote process may still be running (hangup signal received)"), true},
		{"joined noise", errors.Join(context.Canceled, flyerr.Expected(errors.New("the app appears to be crashing"))), true},
		{"joined with a real error", errors.Join(flyerr.Expected(errors.New("the app appears to be crashing")), errors.New("flaps 500")), false},
		{"wrapped multierror with a real error", fmt.Errorf("deploy: %w", multierror.Append(nil, flyerr.Expected(errors.New("the app appears to be crashing")), errors.New("nil pointer dereference"))), false},
		{"wrapped multierror of noise", fmt.Errorf("deploy: %w", multierror.Append(nil, flyerr.Expected(errors.New("the app appears to be crashing")), context.Canceled)), true},
		// Blue-green joins each step's error with a label naming the step.
		{"abort joined with a step label", errors.Join(flyerr.Expected(errors.New("deployment aborted by user")), stepLabel), true},
		{"crash joined with a step label", errors.Join(flyerr.Expected(errors.New("the app appears to be crashing")), stepLabel), true},
		{"signal cause joined with a step label", errors.Join(errors.New(`Get "https://api.machines.dev/v1/apps/a/machines": interrupt signal received`), stepLabel), true},
		{"real error joined with a step label", errors.Join(errors.New("flaps 500"), stepLabel), false},
		{"step label alone is reported", stepLabel, false},
		{"joined step labels alone are reported", errors.Join(stepLabel, stepLabel), false},
		{"cancellation joined with a step label", errors.Join(context.Canceled, stepLabel), true},
		// A pool that cancels its siblings on the first failure joins that
		// failure with their cancellations.
		{"cancelled siblings do not hide the failure", fmt.Errorf("deploy: %w", errors.Join(errors.New("flaps 500"), context.Canceled, context.Canceled)), false},
		// A mark outside a combined error covers all of it.
		{"marked expected around a join", flyerr.Expected(errors.Join(errors.New("a"), errors.New("b"))), true},
		{"expected part does not speak for a join", fmt.Errorf("deploy: %w", errors.Join(flyerr.Expected(errors.New("the app appears to be crashing")), errors.New("flaps 500"))), false},
		{"nil pointer", errors.New("runtime error: invalid memory address or nil pointer dereference"), false},
		{"api timeout", errors.New(`Error contacting Fly.io API when probing "personal": timed out (context deadline exceeded)`), false},
		{"handshake", errors.New("error connecting to SSH server: ssh: handshake failed: EOF"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ignore(tc.err))
		})
	}
}

// The app-info helpers report only the FlapsError inside an error. Filtering
// that narrowed error again dropped events whose other parts were real.
func TestAppInfoHelpersFilterTheWholeErrorOnce(t *testing.T) {
	var sent []string
	client, err := getsentry.NewClient(getsentry.ClientOptions{BeforeSend: func(event *getsentry.Event, _ *getsentry.EventHint) *getsentry.Event {
		var text []string
		for _, e := range event.Exception {
			text = append(text, e.Value)
		}
		sent = append(sent, strings.Join(text, " | "))

		return nil
	}})
	require.NoError(t, err)
	hub := getsentry.CurrentHub()
	previous := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previous) })

	limit := &flaps.FlapsError{OriginalError: errors.New("Your organization has reached its machine limit"), ResponseStatusCode: 422}
	internal := &flaps.FlapsError{OriginalError: errors.New("internal error"), ResponseStatusCode: 500}

	for _, tc := range []struct {
		name string
		err  error
		want string // text the one event must carry; "" for no event
	}{
		{"machine limit alone", fmt.Errorf("launch: %w", limit), ""},
		{"machine limit beside a real failure", errors.Join(errors.New("nil pointer dereference"), limit), "nil pointer dereference"},
		{"machine limit before a flaps 500", errors.Join(limit, internal), "internal error"},
		{"a real flaps error is still reported on its own", fmt.Errorf("launch: %w", internal), "internal error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, report := range []func(){
				func() { CaptureExceptionWithFlapsAppInfo(context.Background(), tc.err, "test", &flaps.App{}) },
				func() {
					CaptureExceptionWithAppInfo(context.Background(), tc.err, "test", &fly.AppCompact{Organization: &fly.OrganizationBasic{}})
				},
			} {
				sent = nil
				report()

				if tc.want == "" {
					require.Empty(t, sent)

					continue
				}
				require.Len(t, sent, 1)
				require.Contains(t, sent[0], tc.want)
			}
		})
	}
}
