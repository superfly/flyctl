package flag

import (
	"context"

	"github.com/spf13/pflag"
)

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
