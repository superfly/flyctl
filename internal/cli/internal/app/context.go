package app

import (
	"context"
)

type contextKey struct{}

// NewContext derives a context that carries cfg from ctx.
func NewContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext returns the Config ctx carries.
func FromContext(ctx context.Context) *Config {
	return ctx.Value(contextKey{}).(*Config)
}
