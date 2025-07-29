package config

import (
	"context"

	"github.com/superfly/fly-go/tokens"
)

type contextKey struct{}

// NewContext derives a Context that carries the given Config from ctx.
func NewContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext returns the Config ctx carries. It panics in case ctx carries
// no Config.
func FromContext(ctx context.Context) *Config {
	return ctx.Value(contextKey{}).(*Config)
}

func Tokens(ctx context.Context) *tokens.Tokens {
	return FromContext(ctx).Tokens
}
