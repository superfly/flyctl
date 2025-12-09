package launch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
)

func TestIsComputeValid(t *testing.T) {
	tests := []struct {
		name     string
		compute  *appconfig.Compute
		expected bool
	}{
		{
			name:     "nil compute",
			compute:  nil,
			expected: false,
		},
		{
			name: "compute with nil MachineGuest",
			compute: &appconfig.Compute{
				MachineGuest: nil,
			},
			expected: false,
		},
		{
			name: "valid compute with MachineGuest",
			compute: &appconfig.Compute{
				MachineGuest: &fly.MachineGuest{
					CPUKind:  "shared",
					CPUs:     1,
					MemoryMB: 256,
				},
			},
			expected: true,
		},
		{
			name: "valid compute with empty MachineGuest",
			compute: &appconfig.Compute{
				MachineGuest: &fly.MachineGuest{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComputeValid(tt.compute)
			assert.Equal(t, tt.expected, result)
		})
	}
}
