package machine

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
)

func TestGetProcessGroup(t *testing.T) {

	type testcase struct {
		name     string
		machine  *api.Machine
		expected string
	}

	cases := []testcase{
		{
			name:     "machine with only 'process_group'",
			expected: "web",
			machine: &api.Machine{

				Config: &api.MachineConfig{
					Metadata: map[string]string{
						"process_group": "web",
					},
				},
			},
		},
		{
			name:     "machine with both 'process_group' & 'fly_process_group'",
			expected: "app",
			machine: &api.Machine{

				Config: &api.MachineConfig{
					Metadata: map[string]string{
						"process_group":     "web",
						"fly_process_group": "app",
					},
				},
			},
		},
		{
			name:     "machine with only 'fly_process_group'",
			expected: "web",
			machine: &api.Machine{

				Config: &api.MachineConfig{
					Metadata: map[string]string{
						"fly_process_group": "web",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		require.Equal(t, tc.expected, getProcessGroup(tc.machine), tc.name)
	}

}
