package launch

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
)

func TestIsReleaseCommandMachine(t *testing.T) {

	type testcase struct {
		name     string
		machines []*api.Machine
		expected bool
	}

	cases := []testcase{
		{
			name:     "machines app, running",
			expected: true,
			machines: []*api.Machine{
				{
					State: "started",
					Checks: []*api.MachineCheckStatus{{
						Status: "passing",
					}},
				},
			},
		},
		{
			name:     "machines app, not running",
			expected: false,
			machines: []*api.Machine{
				{
					State: "started",
					Checks: []*api.MachineCheckStatus{{
						Status: "warning",
					}},
				},
			},
		},
	}

	for _, tc := range cases {
		require.Equal(t, tc.expected, areMachinesRunning(tc.machines), tc.name)
	}

}
