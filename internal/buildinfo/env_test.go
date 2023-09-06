package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDev(t *testing.T) {
	environment = "development"
	assert.True(t, IsDev())
}
