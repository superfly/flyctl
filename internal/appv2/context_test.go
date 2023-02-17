package appv2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigFromContextReturnsNil(t *testing.T) {
	assert.Nil(t, ConfigFromContext(context.Background()))
}

func TestConfigFromContext(t *testing.T) {
	exp := new(Config)

	ctx := WithConfig(context.Background(), exp)
	assert.Same(t, exp, ConfigFromContext(ctx))
}

func TestNameFromContextReturnsEmptyString(t *testing.T) {
	assert.Equal(t, "", NameFromContext(context.Background()))
}

func TestNameFromContext(t *testing.T) {
	const exp = "123"

	ctx := WithName(context.Background(), exp)
	assert.Equal(t, exp, NameFromContext(ctx))
}
