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

// Recover recovers from runtime panics and notifies Sentry.
//
// It reports whether a runtime panic occured.
func Recover() bool {
	switch v := recover(); {
	case v == nil:
		return false
	case initError != nil:
		fmt.Fprintf(os.Stderr, "sentry.Init: %v\n", initError)
	default:
		_ = sentry.CurrentHub().Recover(v)

		printError(v)
	}

	return true
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
