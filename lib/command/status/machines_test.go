package status

import (
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
)

func TestGetImage(t *testing.T) {
	type testcase struct {
		name     string
		machines []*fly.Machine
		expected string
	}

	cases := []testcase{
		{
			name:     "2 machines with different images",
			expected: "library/redis:latest",
			machines: []*fly.Machine{
				{
					ImageRef: fly.MachineImageRef{
						Repository: "library/redis",
						Tag:        "latest",
					},
					Config: &fly.MachineConfig{
						Metadata: map[string]string{
							fly.MachineConfigMetadataKeyFlyReleaseVersion: "2",
						},
					},
				},
				{
					ImageRef: fly.MachineImageRef{
						Repository: "library/nginx",
						Tag:        "latest",
					},
					Config: &fly.MachineConfig{
						Metadata: map[string]string{
							fly.MachineConfigMetadataKeyFlyReleaseVersion: "1",
						},
					},
				},
			},
		},
		{
			name:     "2 machines with same images",
			expected: "library/nginx:latest",
			machines: []*fly.Machine{
				{
					ImageRef: fly.MachineImageRef{
						Repository: "library/nginx",
						Tag:        "latest",
					},
					Config: &fly.MachineConfig{
						Metadata: map[string]string{
							fly.MachineConfigMetadataKeyFlyReleaseVersion: "2",
						},
					},
				},
				{
					ImageRef: fly.MachineImageRef{
						Repository: "library/nginx",
						Tag:        "latest",
					},
					Config: &fly.MachineConfig{
						Metadata: map[string]string{
							fly.MachineConfigMetadataKeyFlyReleaseVersion: "1",
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
