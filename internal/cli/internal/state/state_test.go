package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkingDirectoryPanics(t *testing.T) {
	assert.Panics(t, func() { _ = WorkingDirectory(context.Background()) })
}

func TestWorkingDirectory(t *testing.T) {
	const exp = "workDir"

	ctx := WithWorkingDirectory(context.Background(), exp)
	assert.Equal(t, exp, WorkingDirectory(ctx))
}

func TestUserDirectoryPanics(t *testing.T) {
	assert.Panics(t, func() { _ = UserHomeDirectory(context.Background()) })
}

func TestUserDirectory(t *testing.T) {
	const exp = "userDir"

	ctx := WithUserHomeDirectory(context.Background(), exp)
	assert.Equal(t, exp, UserHomeDirectory(ctx))
}

func TestConfigDirectoryPanics(t *testing.T) {
	assert.Panics(t, func() { _ = ConfigDirectory(context.Background()) })
}

func TestConfigDirectory(t *testing.T) {
	const exp = "configDir"

	ctx := WithConfigDirectory(context.Background(), exp)
	assert.Equal(t, exp, ConfigDirectory(ctx))
}

func TestAccessTokenPanics(t *testing.T) {
	assert.Panics(t, func() { _ = AccessToken(context.Background()) })
}

func TestAccessToken(t *testing.T) {
	const exp = "accessToken"

	ctx := WithAccessToken(context.Background(), exp)
	assert.Equal(t, exp, AccessToken(ctx))
}
