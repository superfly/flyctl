package statuslogger

import (
	"context"
	"fmt"
	"os"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/iostreams"
)

type contextKey struct{}

// NewContext derives a Context that carries sl from ctx.
func NewContext(ctx context.Context, sl StatusLine) context.Context {
	if buildinfo.IsDev() && FromContextOptional(ctx) != nil && os.Getenv("FLYCTL_STATUSLOGGER_NO_ERROR") == "" {
		panic("attempting to create a new statuslogger context when the parent context already has a logger! This is a bug.")
	}
	return context.WithValue(ctx, contextKey{}, sl)
}

// FromContext returns the StatusLine ctx carries. It panics in case ctx carries
// no StatusLine.
func FromContext(ctx context.Context) StatusLine {
	return ctx.Value(contextKey{}).(StatusLine)
}

// FromContextOptional returns the StatusLine ctx carries if any, or nil.
func FromContextOptional(ctx context.Context) StatusLine {
	val := ctx.Value(contextKey{})
	if val == nil {
		return nil
	}
	return ctx.Value(contextKey{}).(StatusLine)
}

// Shorthands for FromContext(ctx).Foo()

func releaseFallback(ctx context.Context, format string, args ...interface{}) bool {
	// For debug builds, we'll let logging functions crash if we haven't set up a status logger.
	// In release builds, we'll log to stdout instead of crashing so we don't break things for users.
	//
	// In a perfect world, this would be a Very Temporary hack, but it'll probably stick around a while just in case.

	if FromContextOptional(ctx) == nil {
		if buildinfo.IsRelease() || os.Getenv("FLYCTL_STATUSLOGGER_NO_ERROR") != "" {
			// TODO(Ali): It'd probably be good to have metrics or sentry here.
			fmt.Fprintf(iostreams.FromContext(ctx).Out, format+"\n", args...)
			return true
		} else {
			panic("Tried to log to a status logger that doesn't exist! This is a bug and crashes debug builds.\nUse FLYCTL_STATUSLOGGER_NO_ERROR=1 to ignore this for now.")
		}
	}
	return false
}

// Log into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func Log(ctx context.Context, s string) {
	if releaseFallback(ctx, "%s", s) {
		return
	}
	FromContext(ctx).Log(s)
}

// Logf into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func Logf(ctx context.Context, format string, args ...interface{}) {
	if releaseFallback(ctx, format, args...) {
		return
	}
	FromContext(ctx).Logf(format, args...)
}

// LogStatus into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func LogStatus(ctx context.Context, s Status, str string) {
	if releaseFallback(ctx, "%s", s) {
		return
	}
	FromContext(ctx).LogStatus(s, str)
}

// LogfStatus into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func LogfStatus(ctx context.Context, s Status, format string, args ...interface{}) {
	if releaseFallback(ctx, format, args...) {
		return
	}
	FromContext(ctx).LogfStatus(s, format, args...)
}

// Failed marks the current line as failed, and prints *the first line* of the error provided.
// The assumption is that the full error will be printed elsewhere.
// Panics if ctx doesn't contain a StatusLine.
func Failed(ctx context.Context, e error) {
	if releaseFallback(ctx, "") {
		return
	}
	FromContext(ctx).Failed(e)
}

// Pause clears the status lines and prevents redraw until the returned resume function is called.
// This allows you to write multiple lines to the terminal without overlapping the status area.
func Pause(ctx context.Context) (ret ResumeFn) {
	ret = func() {}

	line := FromContextOptional(ctx)
	if line == nil {
		return
	}

	if il, ok := line.(*interactiveLine); ok {
		return il.logger.Pause()
	}
	return
}
