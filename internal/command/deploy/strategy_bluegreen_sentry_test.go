package deploy

import (
	"context"
	"errors"
	"testing"

	getsentry "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/sentry"
)

// Deploy joins each step's error with a sentinel naming the step. The label
// must not turn an abort, a cancellation or a crashing app into a Sentry
// event, and must not hide a real failure either.
func TestBlueGreenStepLabelsAndSentry(t *testing.T) {
	var captured int
	client, err := getsentry.NewClient(getsentry.ClientOptions{BeforeSend: func(*getsentry.Event, *getsentry.EventHint) *getsentry.Event { captured++; return nil }})
	require.NoError(t, err)
	hub := getsentry.CurrentHub()
	previous := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previous) })

	labels := map[string]error{
		"ErrCreateGreenMachine":  ErrCreateGreenMachine,
		"ErrWaitForStartedState": ErrWaitForStartedState,
		"ErrWaitForHealthy":      ErrWaitForHealthy,
		"ErrMarkReadyForTraffic": ErrMarkReadyForTraffic,
		"ErrTagForDeletion":      ErrTagForDeletion,
		"ErrCordonBlueMachines":  ErrCordonBlueMachines,
		"ErrStopBlueMachines":    ErrStopBlueMachines,
		"ErrWaitForStoppedState": ErrWaitForStoppedState,
		"ErrDestroyBlueMachines": ErrDestroyBlueMachines,
	}

	count := func(err error) int {
		captured = 0
		sentry.CaptureException(err)

		return captured
	}

	for name, label := range labels {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, 0, count(errors.Join(ErrAborted, label)), "abort")
			require.Equal(t, 0, count(errors.Join(context.Canceled, label)), "cancellation")
			require.Equal(t, 0, count(errors.Join(flyerr.Expected(errors.New("the app appears to be crashing")), label)), "crashing app")
			require.Equal(t, 1, count(errors.Join(errors.New("flaps 500"), label)), "real failure")
			require.Equal(t, 1, count(errors.Join(ErrWaitTimeout, label)), "wait timeout")
		})
	}
}
