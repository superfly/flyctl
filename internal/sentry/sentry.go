// Package sentry implements sentry-related functionality.
package sentry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
	"go.opentelemetry.io/otel/trace"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/buildinfo"
)

var initError error // set during init

// Re-export an alias here for use convenience
type Context = sentry.Context

func init() {
	// Set the timeout on the default HTTPSyncTransport to 3 seconds
	// This is used over initializing the struct directly as we can't
	// set non-exported fields such as transport.limits to a non-nil
	// value, which can result in panics in sentry-go.
	transport := sentry.NewHTTPSyncTransport()
	transport.Timeout = 3 * time.Second

	opts := sentry.ClientOptions{
		Dsn: "https://89fa584dc19b47a6952dd94bf72dbab4@sentry.io/4492967",
		// TODO: maybe set Debug to buildinfo.IsDev?
		// Debug:       true,
		Environment: buildinfo.Environment(),
		Release:     "v" + buildinfo.Version().String(),
		Transport:   transport,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if buildinfo.IsDev() {
				return nil
			}

			return event
		},
	}

	initError = sentry.Init(opts)
}

type CaptureOption func(scope *sentry.Scope)

func WithExtra(key string, val interface{}) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetExtra(key, val)
	}
}

func WithContext(key string, val sentry.Context) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetContext(key, val)
	}
}

func WithContexts(contexts map[string]sentry.Context) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetContexts(contexts)
	}
}

func WithTag(key, value string) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetTag(key, value)
	}
}

func WithTraceID(ctx context.Context) CaptureOption {
	return func(scope *sentry.Scope) {
		currentSpan := trace.SpanFromContext(ctx)
		if currentSpan.SpanContext().HasTraceID() {
			scope.SetTag("trace_id", currentSpan.SpanContext().TraceID().String())
		}
	}
}

func WithStatusCode(status int) CaptureOption {
	return WithTag("status_code", fmt.Sprintf("%d", status))
}

func WithRequestID(requestID string) CaptureOption {
	return WithTag("request_id", requestID)
}

func CaptureException(err error, opts ...CaptureOption) {
	if !isInitialized() {
		return
	}

	sentry.WithScope(func(s *sentry.Scope) {
		for _, opt := range opts {
			opt(s)
		}

		_ = sentry.CaptureException(err)
	})
}

func CaptureMessage(msg string, opts ...CaptureOption) {
	if !isInitialized() {
		return
	}

	sentry.WithScope(func(s *sentry.Scope) {
		for _, opt := range opts {
			opt(s)
		}

		_ = sentry.CaptureMessage(msg)
	})
}

func CaptureExceptionWithAppInfo(ctx context.Context, err error, featureName string, appCompact *fly.AppCompact) {
	if appCompact == nil {
		CaptureException(
			err,
			WithTag("feature", featureName),
		)
		return
	}

	var flapsErr *flaps.FlapsError

	if errors.As(err, &flapsErr) {
		CaptureException(
			flapsErr,
			WithTag("feature", featureName),
			WithTag("app-platform-version", appCompact.PlatformVersion),
			WithContexts(map[string]sentry.Context{
				"app": map[string]interface{}{
					"name": appCompact.Name,
				},
				"organization": map[string]interface{}{
					"slug": appCompact.Organization.Slug,
				},
			}),
			WithTraceID(ctx),
			WithRequestID(flapsErr.FlyRequestId),
			WithStatusCode(flapsErr.ResponseStatusCode),
		)
		return
	}

	CaptureException(
		err,
		WithTag("feature", featureName),
		WithTag("app-platform-version", appCompact.PlatformVersion),
		WithContexts(map[string]sentry.Context{
			"app": map[string]interface{}{
				"name": appCompact.Name,
			},
			"organization": map[string]interface{}{
				"slug": appCompact.Organization.Slug,
			},
		}),
		WithTraceID(ctx),
	)
}

// Recover records the given panic to sentry.
func Recover(v interface{}) {
	if !isInitialized() {
		return
	}

	_ = sentry.CurrentHub().Recover(v)

	printError(v)
}

func printError(v interface{}) {
	var buf bytes.Buffer

	fmt.Fprintln(&buf, aurora.Red("Oops, something went wrong! Could you try that again?"))

	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, v)
	fmt.Fprintln(&buf, string(debug.Stack()))

	buf.WriteTo(os.Stdout)
}

func isInitialized() bool {
	if initError != nil {
		fmt.Fprintf(os.Stderr, "sentry.Init: %v\n", initError)

		return false
	}

	return true
}

func Flush() {
	if !isInitialized() {
		return
	}

	_ = sentry.Flush(time.Second << 1)
}
