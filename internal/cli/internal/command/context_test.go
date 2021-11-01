package command

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestFromContextPanics(t *testing.T) {
	assert.Panics(t, func() { _ = FromContext(context.Background()) })
}

func TestNewContext(t *testing.T) {
	exp := new(cobra.Command)

	ctx := NewContext(context.Background(), exp)
	assert.Equal(t, exp, FromContext(ctx))
}
