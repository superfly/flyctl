package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
)

// Usual Config response for api.GetConfig GQL call
var GetConfigJSON = []byte(`
{
  "env": {},
  "experimental": {
    "auto_rollback": true
  },
  "kill_signal": "SIGINT",
  "kill_timeout": 5,
  "processes": [],
  "services": [
    {
      "concurrency": {
        "hard_limit": 25,
        "soft_limit": 20,
        "type": "connections"
      },
      "http_checks": [],
      "internal_port": 8080,
      "ports": [
        {
          "force_https": true,
          "handlers": [
            "http"
          ],
          "port": 80
        },
        {
          "handlers": [
            "tls",
            "http"
          ],
          "port": 443
        }
      ],
      "processes": [
        "app"
      ],
      "protocol": "tcp",
      "script_checks": [],
      "tcp_checks": [
        {
          "grace_period": "1s",
          "interval": "15s",
          "restart_limit": 0,
          "timeout": "2s"
        }
      ]
    }
  ]
}
`)

func TestFromDefinition(t *testing.T) {
	definition := &api.Definition{}
	err := json.Unmarshal(GetConfigJSON, definition)
	assert.NoError(t, err)

	cfg, err := FromDefinition(definition)
	assert.NoError(t, err)

	assert.Equal(t, &Config{
		KillSignal:  "SIGINT",
		KillTimeout: 5,
		Experimental: &Experimental{
			AutoRollback: true,
		},
		Env: map[string]string{},
		Services: []Service{
			{
				InternalPort: 8080,
				Protocol:     "tcp",
				Concurrency: &api.MachineServiceConcurrency{
					Type:      "connections",
					HardLimit: 25,
					SoftLimit: 20,
				},
				Ports: []api.MachinePort{
					{
						Port:       api.Pointer(80),
						Handlers:   []string{"http"},
						ForceHttps: true,
					},
					{
						Port:     api.Pointer(443),
						Handlers: []string{"tls", "http"},
					},
				},
				Processes: []string{"app"},
				TCPChecks: []*ServiceTCPCheck{
					{
						Timeout:      api.MustParseDuration("2s"),
						RestartLimit: 0,
						Interval:     api.MustParseDuration("15s"),
						GracePeriod:  api.MustParseDuration("1s"),
					},
				},
			},
		},
	}, cfg)
}

func TestToDefinition(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)

	definition, err := cfg.ToDefinition()
	assert.NoError(t, err)
	assert.Equal(t, &api.Definition{
		"kill_signal":    "SIGTERM",
		"kill_timeout":   int64(3),
		"primary_region": "sea",
		"experimental": map[string]any{
			"cmd":           []any{"cmd"},
			"entrypoint":    []any{"entrypoint"},
			"exec":          []any{"exec"},
			"auto_rollback": true,
			"enable_consul": true,
			"enable_etcd":   true,
		},

		"deploy": map[string]any{
			"release_command": "release command",
			"strategy":        "rolling-eyes",
		},
		"env": map[string]any{
			"FOO": "BAR",
		},
		"metrics": map[string]any{
			"port": int64(9999),
			"path": "/metrics",
		},
		"http_service": map[string]any{
			"internal_port": int64(8080),
			"force_https":   true,
			"concurrency": map[string]any{
				"type":       "donuts",
				"hard_limit": int64(10),
				"soft_limit": int64(4),
			},
		},
		"statics": []map[string]any{
			{
				"guest_path": "/path/to/statics",
				"url_prefix": "/static-assets",
			},
		},
		"mounts": map[string]any{
			"source":      "data",
			"destination": "/data",
		},
		"processes": map[string]any{
			"web":  "run web",
			"task": "task all day",
		},
		"checks": map[string]any{
			"status": map[string]any{
				"port":            int64(2020),
				"type":            "http",
				"interval":        "10s",
				"timeout":         "2s",
				"method":          "GET",
				"path":            "/status",
				"protocol":        "https",
				"tls_skip_verify": true,
				"headers": map[string]any{
					"Content-Type":  "application/json",
					"Authorization": "super-duper-secret",
				},
			},
		},
		"services": []map[string]any{
			{
				"internal_port": int64(8081),
				"protocol":      "tcp",
				"processes":     []any{"app"},
				"concurrency": map[string]any{
					"type":       "requests",
					"hard_limit": int64(22),
					"soft_limit": int64(13),
				},
				"ports": []map[string]any{
					{
						"port":        int64(80),
						"start_port":  int64(100),
						"end_port":    int64(200),
						"handlers":    []any{"https"},
						"force_https": true,
					},
				},
				"tcp_checks": []map[string]any{
					{
						"interval": "21s",
						"timeout":  "4s",
					},
				},
				"http_checks": []map[string]any{
					{
						"interval":        "1m21s",
						"timeout":         "7s",
						"method":          "GET",
						"path":            "/",
						"protocol":        "https",
						"tls_skip_verify": true,
						"headers": map[string]any{
							"My-Custom-Header": "whatever",
						},
					},
					{
						"interval": "33s",
						"timeout":  "10s",
						"method":   "POST",
						"path":     "/check2",
					},
				},
			},
		},
	}, definition)
}
