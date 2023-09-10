package set

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

func TestSet(t *testing.T) {

	var mySet Set[string]

	assert.False(t, mySet.HasAny("hello", "world"))

	mySet.Set("hello", "world")

	assert.True(t, mySet.Has("hello"))
	assert.True(t, mySet.Has("world"))
	assert.False(t, mySet.Has("foo"))

	assert.True(t, mySet.HasAny("hello", "world"))
	assert.True(t, mySet.HasAny("hello", "world", "foo"))

	assert.True(t, mySet.HasAll("hello", "world"))
	assert.False(t, mySet.HasAll("hello", "world", "foo"))

	hwSorted := []string{"hello", "world"}
	slices.Sort(hwSorted)
	valuesSorted := mySet.Values()
	slices.Sort(valuesSorted)
	assert.Equal(t, hwSorted, valuesSorted)

	other := mySet.Copy()
	assert.True(t, other.Has("hello"))
	other.Set("foo")
	assert.False(t, mySet.Has("foo"))

	assert.Equal(t, 3, other.Len())

	mySet.Clear()

	assert.False(t, mySet.HasAny("hello", "world"))

}
