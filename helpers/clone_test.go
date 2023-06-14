package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type child struct {
	Name string
}

type cloneable struct {
	A string
	B int
	C float32
	D child
}

func TestClone(t *testing.T) {
	cloneMe := cloneable{
		A: "abcd",
		B: 123,
		C: 1.5,
		D: child{
			Name: "aname",
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

func TestClonePointer(t *testing.T) {

	type child struct {
		S string
	}
	type cloned struct {
		Ch *child
	}

	c := cloned{
		Ch: &child{
			S: "hello",
		},
	}

	clonedObj := Clone(c)

	c.Ch.S = "modified"

	assert.NotEqualValues(t, c.Ch.S, clonedObj.Ch.S)
}

func TestCloneMap(t *testing.T) {
	cloneMe := map[string]int{
		"one": 1,
		"two": 2,
	}

	cloned := Clone(cloneMe)

	assert.EqualValues(t, cloneMe, cloned)

	cloned["two"]++

	assert.Equal(t, cloneMe["two"], 2)
	assert.Equal(t, cloned["two"], 3)
}
