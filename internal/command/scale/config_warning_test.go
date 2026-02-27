package scale

import (
	"testing"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"

	"github.com/stretchr/testify/assert"
)

func TestResolveComputeSettings(t *testing.T) {
	testcases := []struct {
		name           string
		compute        *appconfig.Compute
		expectedSize   string
		expectedMemory int
	}{
		{
			name: "explicit size returns canonical name and default memory",
			compute: &appconfig.Compute{
				Size: "shared-cpu-1x",
			},
			expectedSize:   "shared-cpu-1x",
			expectedMemory: 256,
		},
		{
			name: "performance size returns correct defaults",
			compute: &appconfig.Compute{
				Size: "performance-2x",
			},
			expectedSize:   "performance-2x",
			expectedMemory: 4096,
		},
		{
			name: "size with memory string override",
			compute: &appconfig.Compute{
				Size:   "shared-cpu-1x",
				Memory: "512mb",
			},
			expectedSize:   "shared-cpu-1x",
			expectedMemory: 512,
		},
		{
			name:    "no size defaults to shared-cpu-1x",
			compute: &appconfig.Compute{},
			expectedSize:   "shared-cpu-1x",
			expectedMemory: 256,
		},
		{
			name: "inline MachineGuest.MemoryMB override",
			compute: &appconfig.Compute{
				Size: "shared-cpu-1x",
				MachineGuest: &fly.MachineGuest{
					MemoryMB: 1024,
				},
			},
			expectedSize:   "shared-cpu-1x",
			expectedMemory: 1024,
		},
		{
			name: "GPU kind defaults to performance-8x size",
			compute: &appconfig.Compute{
				MachineGuest: &fly.MachineGuest{
					GPUKind: "a100-pcie-40gb",
				},
			},
			expectedSize:   "performance-8x",
			expectedMemory: 16384,
		},
		{
			name: "memory string takes precedence over size default but inline MemoryMB wins",
			compute: &appconfig.Compute{
				Size:   "shared-cpu-1x",
				Memory: "512mb",
				MachineGuest: &fly.MachineGuest{
					MemoryMB: 1024,
				},
			},
			expectedSize:   "shared-cpu-1x",
			expectedMemory: 1024,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			sizeName, memoryMB := resolveComputeSettings(tc.compute)
			assert.Equal(t, tc.expectedSize, sizeName)
			assert.Equal(t, tc.expectedMemory, memoryMB)
		})
	}
}
