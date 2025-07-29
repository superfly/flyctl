package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
)

func TestIsFlex(t *testing.T) {
	assert.False(t, IsFlex(nil))
	assert.False(t, IsFlex(&fly.Machine{}))
	assert.False(t, IsFlex(&fly.Machine{
		ImageRef: fly.MachineImageRef{
			Labels: map[string]string{
				"fly.pg-manager": "stolon",
			},
		},
	}))
	assert.True(t, IsFlex(&fly.Machine{
		ImageRef: fly.MachineImageRef{
			Labels: map[string]string{
				"fly.pg-manager": "repmgr",
			},
		},
	}))
}
