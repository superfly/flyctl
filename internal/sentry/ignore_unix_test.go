//go:build unix

package sentry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// Ctrl-C and SIGTERM reach flyctl as the cause of a context made by
// signal.NotifyContext. The filter relies on that cause reporting itself as a
// cancellation rather than on its "interrupt signal received" text, so check
// it against the real thing.
func TestIgnoreSignalCause(t *testing.T) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGUSR1)
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGUSR1))

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("signal not delivered")
	}

	cause := context.Cause(ctx)
	require.Contains(t, cause.Error(), "signal received")

	// The shapes seen in Sentry: an HTTP client error, wrapped once more.
	httpErr := &url.Error{Op: "Post", URL: "https://api.fly.io/api/v1/builds/finish", Err: cause}
	require.True(t, ignore(httpErr))
	require.True(t, ignore(fmt.Errorf("create ssh certificate: %w", httpErr)))
	require.True(t, ignore(errors.Join(httpErr, stepLabel)))
	// GraphQL errors reach Sentry through pkg/errors wrappers.
	require.True(t, ignore(pkgerrors.Wrap(pkgerrors.WithStack(cause), "reading body")))
}
