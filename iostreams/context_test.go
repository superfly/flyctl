package iostreams

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromContextPanics(t *testing.T) {
	assert.Panics(t, func() { _ = FromContext(context.Background()) })
}

func TestNewContext(t *testing.T) {
	exp := new(IOStreams)

	ctx := NewContext(context.Background(), exp)
	assert.Same(t, exp, FromContext(ctx))
}
