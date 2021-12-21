// Package state implements setters and getters for command contexts.
package state

import (
	"context"
	"path/filepath"

	"github.com/superfly/flyctl/internal/cli/internal/config"
)

type contextKeyType int

const (
	_ contextKeyType = iota
	hostnameKey
	workDirKey
	userHomeDirKey
	configDirKey
	accessTokenKey
	appNameKey
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

// WithUserHomeDirectory derives a Context that carries the given user home
// directory from ctx.
func WithUserHomeDirectory(ctx context.Context, uhd string) context.Context {
	return set(ctx, userHomeDirKey, uhd)
}

// UserHomeDirectory returns the user home directory ctx carries. It panics in
// case ctx carries no user home directory.
func UserHomeDirectory(ctx context.Context) string {
	return get(ctx, userHomeDirKey).(string)
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

// ConfigFile returns the config file ctx carries. It panics in case
// ctx carries no config directory.
func ConfigFile(ctx context.Context) string {
	return filepath.Join(ConfigDirectory(ctx), config.FileName)
}

func get(ctx context.Context, key contextKeyType) interface{} {
	return ctx.Value(key)
}

func set(ctx context.Context, key contextKeyType, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
