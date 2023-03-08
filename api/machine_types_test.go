package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsReleaseCommandMachine(t *testing.T) {

	type testcase struct {
		name     string
		machine  Machine
		expected bool
	}

	cases := []testcase{
		{
			name:     "release machine using 'process_group'",
			expected: true,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"process_group": "release_command",
					},
				},
			},
		},
		{
			name:     "release machine using 'fly_process_group'",
			expected: true,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"fly_process_group": "fly_app_release_command",
					},
				},
			},
		},
		{
			name:     "non-release machine using 'fly_process_group'",
			expected: false,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"fly_process_group": "web",
					},
				},
			},
		},
		{
			name:     "non-release machine using 'process_group'",
			expected: false,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"process_group": "web",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		require.Equal(t, tc.expected, tc.machine.IsReleaseCommandMachine(), tc.name)
	}

}
