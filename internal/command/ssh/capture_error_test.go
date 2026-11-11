package ssh

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

func TestCaptureSSHError(t *testing.T) {
	var captured int
	var lastMessage string
	client, err := getsentry.NewClient(getsentry.ClientOptions{BeforeSend: func(event *getsentry.Event, _ *getsentry.EventHint) *getsentry.Event {
		captured++
		lastMessage = event.Exception[len(event.Exception)-1].Value
		return nil
	}})
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
		{"missing exit status", pkgerrors.Wrap(&gossh.ExitMissingError{}, "ssh shell"), 0},
		{"session closed by ctrl-c", pkgerrors.Wrap(fmt.Errorf("session forcibly closed; the remote process may still be running: %w", errors.New("interrupt signal received")), "ssh shell"), 0},
		{"matching text is not sufficient", errors.New("ssh shell: Process exited with status 1"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured = 0
			captureError(context.Background(), tc.err, app)
			require.Equal(t, tc.want, captured)
		})
	}

	t.Run("rejected exec is reported without the command", func(t *testing.T) {
		captured, lastMessage = 0, ""
		cmd := `sqlite3 /data/app.db "SELECT * FROM users WHERE email = 'someone@example.com'"`
		captureError(context.Background(), pkgerrors.Wrap(fmt.Errorf("ssh: command %v failed", cmd), "ssh shell"), app)
		require.Equal(t, 1, captured)
		require.NotContains(t, lastMessage, "sqlite3")
		require.Equal(t, errExecRejected.Error(), lastMessage)
	})
}
