// Package state implements setters and getters for command contexts.
package state

import (
	"context"

	"github.com/superfly/flyctl/api"
)

type contextKeyType int

const (
	_ contextKeyType = iota
	workDirKey
	userHomeDirKey
	configDirKey
	accessTokenKey
	orgKey
)

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

// WithOrg derives a Context that carries the given Organization from ctx.
func WithOrg(ctx context.Context, org *api.Organization) context.Context {
	return set(ctx, orgKey, org)
}

// Org returns the Organization ctx carries. It panics in case ctx carries no
// Organization.
func Org(ctx context.Context) *api.Organization {
	return get(ctx, orgKey).(*api.Organization)
}

func get(ctx context.Context, key contextKeyType) interface{} {
	return ctx.Value(key)
}

func set(ctx context.Context, key contextKeyType, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
