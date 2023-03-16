package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
)

func TestIsFlex(t *testing.T) {
	assert.False(t, IsFlex(nil))
	assert.False(t, IsFlex(&api.Machine{}))
	assert.False(t, IsFlex(&api.Machine{
		ImageRef: api.MachineImageRef{
			Labels: map[string]string{
				"fly.pg-manager": "stolon",
			},
		},
	}))
	assert.True(t, IsFlex(&api.Machine{
		ImageRef: api.MachineImageRef{
			Labels: map[string]string{
				"fly.pg-manager": "repmgr",
			},
		},
	}))
}
