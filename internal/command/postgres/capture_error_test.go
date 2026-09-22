package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	getsentry "github.com/getsentry/sentry-go"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	gossh "golang.org/x/crypto/ssh"
)

func TestCaptureBarmanError(t *testing.T) {
	var captured int
	client, err := getsentry.NewClient(getsentry.ClientOptions{BeforeSend: func(event *getsentry.Event, _ *getsentry.EventHint) *getsentry.Event { captured++; return nil }})
	require.NoError(t, err)
	hub := getsentry.CurrentHub()
	previous := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previous) })
	app := &flaps.App{}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"exit error", &gossh.ExitError{}, 0},
		{"wrapped exit error", pkgerrors.Wrap(&gossh.ExitError{}, "ssh shell"), 0},
		{"wrapped cancellation", fmt.Errorf("connect: %w", context.Canceled), 0},
		{"transport", errors.New("connection reset by peer"), 1},
		{"auth", errors.New("ssh: unable to authenticate"), 1},
		{"missing exit status", &gossh.ExitMissingError{}, 1},
		{"matching text is not sufficient", errors.New("ssh shell: Process exited with status 1"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured = 0
			captureError(context.Background(), tc.err, app)
			require.Equal(t, tc.want, captured)
		})
	}
}
