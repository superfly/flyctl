package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromContextDoesNotPanic(t *testing.T) {
	assert.Nil(t, FromContext(context.Background()))
}

func TestNewContext(t *testing.T) {
	exp := new(Config)

	ctx := NewContext(context.Background(), exp)
	assert.Same(t, exp, FromContext(ctx))
}
