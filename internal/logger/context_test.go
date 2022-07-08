package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromContextPanics(t *testing.T) {
	assert.Panics(t, func() { _ = FromContext(context.Background()) })
}

func TestNewContext(t *testing.T) {
	exp := new(Logger)

	ctx := NewContext(context.Background(), exp)
	assert.Same(t, exp, FromContext(ctx))
}

func TestMaybeFromContextDoesNotPanic(t *testing.T) {
	assert.Nil(t, MaybeFromContext(context.Background()))
}

func TestMaybeFromContext(t *testing.T) {
	exp := new(Logger)

	ctx := NewContext(context.Background(), exp)
	assert.Same(t, exp, MaybeFromContext(ctx))

	assert.Nil(t, MaybeFromContext(context.Background()))
}
