package statuslogger

import (
	"context"
)

type contextKey struct{}

// NewContext derives a Context that carries sl from ctx.
func NewContext(ctx context.Context, sl StatusLine) context.Context {
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

// Log into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func Log(ctx context.Context, s string) {
	FromContext(ctx).Log(s)
}

// Logf into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func Logf(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Logf(format, args...)
}

// LogStatus into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func LogStatus(ctx context.Context, s Status, str string) {
	FromContext(ctx).LogStatus(s, str)
}

// LogfStatus into the StatusLine ctx carries. Panics if ctx doesn't contain a StatusLine.
func LogfStatus(ctx context.Context, s Status, format string, args ...interface{}) {
	FromContext(ctx).LogfStatus(s, format, args...)
}
