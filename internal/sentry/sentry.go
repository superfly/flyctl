// Package sentry implements sentry-related functionality.
package sentry

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/internal/buildinfo"
)

var initError error // set during init

func init() {
	opts := sentry.ClientOptions{
		Dsn: "https://89fa584dc19b47a6952dd94bf72dbab4@sentry.io/4492967",
		// TODO: maybe set Debug to buildinfo.IsDev?
		// Debug:       true,
		Environment: buildinfo.Environment(),
		Release:     buildinfo.Version().String(),
		Transport: &sentry.HTTPSyncTransport{
			Timeout: 3 * time.Second,
		},
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

func WithContext(key string, val interface{}) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetContext(key, val)
	}
}

func WithContexts(contexts map[string]interface{}) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetContexts(contexts)
	}
}

func WithTag(key, value string) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetTag(key, value)
	}
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

	if buildinfo.IsDev() {
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, v)
		fmt.Fprintln(&buf, string(debug.Stack()))
	}

	buf.WriteTo(os.Stdout)
}

func isInitialized() bool {
	if initError != nil {
		fmt.Fprintf(os.Stderr, "sentry.Init: %v\n", initError)

		return false
	}

	return true
}
