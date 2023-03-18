package status

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
)

func TestGetImage(t *testing.T) {

	type testcase struct {
		name     string
		machines []*api.Machine
		expected string
	}

	cases := []testcase{
		{
			name:     "2 machines with different images",
			expected: "library/redis:latest",
			machines: []*api.Machine{
				{
					ImageRef: api.MachineImageRef{
						Repository: "library/redis",
						Tag:        "latest",
					},
					Config: &api.MachineConfig{
						Metadata: map[string]string{
							api.MachineConfigMetadataKeyFlyReleaseVersion: "2",
						},
					},
				},
				{
					ImageRef: api.MachineImageRef{
						Repository: "library/nginx",
						Tag:        "latest",
					},
					Config: &api.MachineConfig{
						Metadata: map[string]string{
							api.MachineConfigMetadataKeyFlyReleaseVersion: "1",
						},
					},
				},
			},
		},
		{
			name:     "2 machines with same images",
			expected: "library/nginx:latest",
			machines: []*api.Machine{
				{
					ImageRef: api.MachineImageRef{
						Repository: "library/nginx",
						Tag:        "latest",
					},
					Config: &api.MachineConfig{
						Metadata: map[string]string{
							api.MachineConfigMetadataKeyFlyReleaseVersion: "2",
						},
					},
				},
				{
					ImageRef: api.MachineImageRef{
						Repository: "library/nginx",
						Tag:        "latest",
					},
					Config: &api.MachineConfig{
						Metadata: map[string]string{
							api.MachineConfigMetadataKeyFlyReleaseVersion: "1",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		img, err := getImage(tc.machines)
		require.NoError(t, err)
		require.Equal(t, tc.expected, img, tc.name)
	}

}
