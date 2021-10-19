package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppNamePanics(t *testing.T) {
	assert.Panics(t, func() { _ = AppName(context.Background()) })
}

func TestAppName(t *testing.T) {
	const exp = "appName"

	ctx := WithAppName(context.Background(), exp)
	assert.Equal(t, exp, AppName(ctx))
}
