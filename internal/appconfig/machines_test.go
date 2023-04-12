package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
)

func TestToMachineConfig(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine.toml")
	require.NoError(t, err)

	want := &api.MachineConfig{
		Env: map[string]string{
			"FOO":            "BAR",
			"PRIMARY_REGION": "mia",
		},

		Services: []api.MachineService{
			{
				Protocol:     "tcp",
				InternalPort: 8080,
				Ports: []api.MachinePort{
					{
						Port:       api.Pointer(80),
						Handlers:   []string{"http"},
						ForceHttps: true,
					},
					{
						Port:       api.Pointer(443),
						Handlers:   []string{"http", "tls"},
						ForceHttps: false,
					},
				},
			},
		},

		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "app",
		},

		Metrics: &api.MachineMetrics{
			Port: 9999,
			Path: "/metrics",
		},

		Statics: []*api.Static{{
			GuestPath: "/guest/path",
			UrlPrefix: "/url/prefix",
		}},

		Mounts: []api.MachineMount{{
			Name: "data",
			Path: "/data",
		}},

		Checks: map[string]api.MachineCheck{
			"listening": {
				Port: api.Pointer(8080),
				Type: api.Pointer("tcp"),
			},
			"status": {
				Port:     api.Pointer(8080),
				Type:     api.Pointer("http"),
				Interval: mustParseDuration("10s"),
				Timeout:  mustParseDuration("1s"),
				HTTPPath: api.Pointer("/status"),
			},
		},
	}

	got, err := cfg.ToMachineConfig("", nil)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestToReleaseMachineConfig(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine.toml")
	require.NoError(t, err)

	want := &api.MachineConfig{
		Init: api.MachineInit{
			Cmd: []string{"migrate-db"},
		},

		Env: map[string]string{
			"FOO":             "BAR",
			"PRIMARY_REGION":  "mia",
			"RELEASE_COMMAND": "1",
		},

		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_release_command",
		},

		AutoDestroy: true,
		Restart: api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
		DNS: &api.DNSConfig{
			SkipRegistration: true,
		},
	}

	got, err := cfg.ToReleaseMachineConfig()
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestToMachineConfig_multiProcessGroups(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-processgroups.toml")
	require.NoError(t, err)

	testcases := []struct {
		name      string
		groupName string
		want      *api.MachineConfig
	}{
		{
			name:      "default empty process group",
			groupName: "app",
			want: &api.MachineConfig{
				Init: api.MachineInit{
					Cmd: []string{"run-nginx"},
				},
				Services: []api.MachineService{
					{
						Protocol:     "tcp",
						InternalPort: 8080,
						Ports: []api.MachinePort{
							{Port: api.Pointer(80), Handlers: []string{"http"}},
							{Port: api.Pointer(443), Handlers: []string{"http", "tls"}},
						},
					},
					{Protocol: "tcp", InternalPort: 1111},
				},
				Checks: map[string]api.MachineCheck{
					"listening": {Port: api.Pointer(8080), Type: api.Pointer("tcp")},
				},
			},
		},
		{
			name:      "vpn process group",
			groupName: "vpn",
			want: &api.MachineConfig{
				Init: api.MachineInit{
					Cmd: []string{"run-tailscale"},
				},
				Services: []api.MachineService{
					{Protocol: "udp", InternalPort: 9999},
					{Protocol: "tcp", InternalPort: 1111},
				},
			},
		},
		{
			name:      "foo process group",
			groupName: "foo",
			want: &api.MachineConfig{
				Init: api.MachineInit{
					Cmd: []string{"keep", "me", "alive"},
				},
				Services: []api.MachineService{
					{Protocol: "tcp", InternalPort: 1111},
				},
				Checks: map[string]api.MachineCheck{
					"listening": {Port: api.Pointer(8080), Type: api.Pointer("tcp")},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cfg.ToMachineConfig(tc.groupName, nil)
			require.NoError(t, err)
			// We only care about fields that change for different process groups
			assert.Equal(t, tc.groupName, got.Metadata["fly_process_group"])
			assert.Equal(t, tc.want.Init, got.Init)
			assert.Equal(t, tc.want.Services, got.Services)
			assert.Equal(t, tc.want.Checks, got.Checks)
		})
	}
}
