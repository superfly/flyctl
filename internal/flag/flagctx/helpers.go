package flagctx

import (
	"context"

	"github.com/spf13/pflag"
)

// This is a hack to allow constructing a flag context from within completion,
// a dependency of flag.

type contextKey struct{}

// NewContext derives a context that carries fs from ctx.
func NewContext(ctx context.Context, fs *pflag.FlagSet) context.Context {
	return context.WithValue(ctx, contextKey{}, fs)
}

// FromContext returns the FlagSet ctx carries. It panics in case ctx carries
// no FlagSet.
func FromContext(ctx context.Context) *pflag.FlagSet {
	return ctx.Value(contextKey{}).(*pflag.FlagSet)
}

// FromContextOrNil returns the FlagSet ctx carries, or nil in case ctx carries
// none. It exists for callers that run before flags are guaranteed to be in
// context, such as the preparer that determines the config directory.
func FromContextOrNil(ctx context.Context) *pflag.FlagSet {
	fs, _ := ctx.Value(contextKey{}).(*pflag.FlagSet)

	return fs
}
