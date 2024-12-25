package appconfig

import (
	"context"
)

type contextKeyType int

const (
	_ contextKeyType = iota
	configContextKey
	nameContextKey
	seedContextKey
)

// WithConfig derives a context that carries cfg from ctx.
func WithConfig(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, configContextKey, cfg)
}

// ConfigFromContext returns the Config ctx carries.
func ConfigFromContext(ctx context.Context) *Config {
	if cfg, ok := ctx.Value(configContextKey).(*Config); ok {
		return cfg
	}

	return nil
}

// WithName derives a context that carries the given app name from ctx.
func WithName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, nameContextKey, name)
}

// NameFromContext returns the app name ctx carries or an empty string.
func NameFromContext(ctx context.Context) string {
	if name, ok := ctx.Value(nameContextKey).(string); ok {
		return name
	}

	return ""
}

// WithSeed derives a context that carries the given seed from ctx.
func WithSeedCommand(ctx context.Context, seedCommand string) context.Context {
	return context.WithValue(ctx, seedContextKey, seedCommand)
}

// SeedFromContext returns the seed ctx carries or an empty string.
func SeedCommandFromContext(ctx context.Context) string {
	if seed, ok := ctx.Value(seedContextKey).(string); ok {
		return seed
	}

	return ""
}
