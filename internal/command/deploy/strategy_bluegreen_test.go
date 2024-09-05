package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlueGreenStrategy(t *testing.T) {
	s := BlueGreenStrategy(&machineDeployment{}, nil)
	assert.False(t, s.isAborted())
}
