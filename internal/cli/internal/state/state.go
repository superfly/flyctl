// Package state implements Context.
package state

import (
	"context"
)

type contextKeyType int

const (
	_ contextKeyType = iota
	appNameKey
	workDirKey
	userHomeDirKey
	configDirKey
	configFileKey
	accessTokenKey
	viperKey
)

// WithAppName derives a Context carries the given app name from ctx.
func WithAppName(ctx context.Context, appName string) context.Context {
	return set(ctx, appNameKey, appName)
}

// AppName returns the app name ctx carries. It panics in case ctx carries no
// app name.
func AppName(ctx context.Context) string {
	return get(ctx, appNameKey).(string)
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

// WithConfigFile derives a Context that carries the given config file from
// ctx.
func WithConfigFile(ctx context.Context, cd string) context.Context {
	return set(ctx, configFileKey, cd)
}

// ConfigFile returns the config file ctx carries. It panics in case
// ctx carries no config file.
func ConfigFile(ctx context.Context) string {
	return get(ctx, configFileKey).(string)
}

// WithAccessToken derives a Context that carries the given access token from
// ctx.
func WithAccessToken(ctx context.Context, token string) context.Context {
	return set(ctx, accessTokenKey, token)
}

// AccessToken returns the access token ctx carries. It panics in case ctx
// carries no access token.
func AccessToken(ctx context.Context) string {
	return get(ctx, accessTokenKey).(string)
}

func get(ctx context.Context, key contextKeyType) interface{} {
	return ctx.Value(key)
}

func set(ctx context.Context, key contextKeyType, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
