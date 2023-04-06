package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type child struct {
	name string
}

type cloneable struct {
	a string
	b int
	c float32
	d child
}

func TestClone(t *testing.T) {
	cloneMe := cloneable{
		a: "abcd",
		b: 123,
		c: 1.5,
		d: child{
			name: "aname",
		},
	}

	cloned := Clone(cloneMe)

	assert.Equal(t, cloneMe, cloned)

	cloneMePtr := &cloneMe

	clonedPtr := Clone(cloneMePtr)

	assert.NotEmpty(t, clonedPtr)

	assert.Equal(t, *cloneMePtr, *clonedPtr)

	clonedPtr = nil
	clonedNil := Clone(clonedPtr)

	assert.Empty(t, clonedNil)
}
