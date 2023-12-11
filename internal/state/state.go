// Package state implements setters and getters for command contexts.
package state

import (
	"context"
)

type contextKeyType int

const (
	_ contextKeyType = iota
	hostnameKey
	workDirKey
	configDirKey
	stateDirKey
	runtimeDirKey
)

// WithHostname returns a copy of ctx that carries hostname.
func WithHostname(ctx context.Context, hostname string) context.Context {
	return set(ctx, hostnameKey, hostname)
}

// Hostname returns the hostname ctx carries. It panics in case ctx carries no
// hostname.
func Hostname(ctx context.Context) string {
	return get(ctx, hostnameKey).(string)
}

// WithWorkingDirectory derives a Context that carries the given working
// directory from ctx.
func WithWorkingDirectory(ctx context.Context, wd string) context.Context {
	return set(ctx, workDirKey, wd)
}

// WorkingDirectory returns the working directory ctx carries. It panics in case
// ctx carries no working directory.
func WorkingDirectory(ctx context.Context) string {
	return get(ctx, workDirKey).(string)
}

// WithConfigDir derives a Context that carries the given config directory from
// ctx.
func WithConfigDirectory(ctx context.Context, cd string) context.Context {
	return set(ctx, configDirKey, cd)
}

// ConfigDirectory returns the config directory ctx carries. It panics in case
// ctx carries no config directory.
func ConfigDirectory(ctx context.Context) string {
	return get(ctx, configDirKey).(string)
}

// WithStateDir derives a Context that carries the given state directory from
// ctx.
func WithStateDirectory(ctx context.Context, cd string) context.Context {
	return set(ctx, stateDirKey, cd)
}

// StateDirectory returns the state directory ctx carries. It panics in case
// ctx carries no state directory.
func StateDirectory(ctx context.Context) string {
	return get(ctx, stateDirKey).(string)
}

// WithRuntimeDir derives a Context that carries the given runtime directory from
// ctx.
func WithRuntimeDirectory(ctx context.Context, cd string) context.Context {
	return set(ctx, runtimeDirKey, cd)
}

// RuntimeDirectory returns the runtime directory ctx carries. It panics in case
// ctx carries no runtime directory.
func RuntimeDirectory(ctx context.Context) string {
	return get(ctx, runtimeDirKey).(string)
}

func get(ctx context.Context, key contextKeyType) interface{} {
	return ctx.Value(key)
}

func set(ctx context.Context, key contextKeyType, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
