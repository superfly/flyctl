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
				InternalPort: 0,
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

	got, err := cfg.ToMachineConfig("")
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
