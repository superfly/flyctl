package appconfig

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		KillSignal:  api.Pointer("SIGINT"),
		KillTimeout: api.MustParseDuration("5s"),
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
						ForceHTTPS: true,
					},
					{
						Port:     api.Pointer(443),
						Handlers: []string{"tls", "http"},
					},
				},
				Processes: []string{"app"},
				TCPChecks: []*ServiceTCPCheck{
					{
						Timeout:     api.MustParseDuration("2s"),
						Interval:    api.MustParseDuration("15s"),
						GracePeriod: api.MustParseDuration("1s"),
					},
				},
			},
		},
		configFilePath:   "--config path unset--",
		defaultGroupName: "app",
		RawDefinition: map[string]any{
			"env": map[string]any{},
			"experimental": map[string]any{
				"auto_rollback": true,
			},
			"kill_signal":  "SIGINT",
			"kill_timeout": float64(5),
			"processes":    []any{},
			"services": []any{
				map[string]any{
					"concurrency": map[string]any{
						"hard_limit": float64(25),
						"soft_limit": float64(20),
						"type":       "connections",
					},
					"http_checks":   []any{},
					"internal_port": float64(8080),
					"ports": []any{
						map[string]any{
							"force_https": true,
							"handlers":    []any{"http"},
							"port":        float64(80),
						},
						map[string]any{
							"handlers": []any{"tls", "http"},
							"port":     float64(443),
						},
					},
					"processes":     []any{"app"},
					"protocol":      "tcp",
					"script_checks": []any{},
					"tcp_checks": []any{
						map[string]any{
							"grace_period":  "1s",
							"interval":      "15s",
							"restart_limit": float64(0),
							"timeout":       "2s",
						},
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
		"app":                "foo",
		"primary_region":     "sea",
		"kill_signal":        "SIGTERM",
		"kill_timeout":       "3s",
		"swap_size_mb":       int64(512),
		"console_command":    "/bin/bash",
		"host_dedication_id": "06031957",
		"build": map[string]any{
			"builder":      "dockerfile",
			"image":        "foo/fighter",
			"builtin":      "whatisthis",
			"dockerfile":   "Dockerfile",
			"ignorefile":   ".gitignore",
			"build-target": "target",
			"buildpacks":   []any{"packme", "well"},
			"settings": map[string]any{
				"foo":   "bar",
				"other": float64(2),
			},
			"args": map[string]any{
				"param1": "value1",
				"param2": "value2",
			},
		},

		"http_service": map[string]any{
			"internal_port":        int64(8080),
			"force_https":          true,
			"auto_start_machines":  false,
			"auto_stop_machines":   false,
			"min_machines_running": int64(0),
			"concurrency": map[string]any{
				"type":       "donuts",
				"hard_limit": int64(10),
				"soft_limit": int64(4),
			},
			"tls_options": map[string]any{
				"alpn":                []any{"h2", "http/1.1"},
				"versions":            []any{"TLSv1.2", "TLSv1.3"},
				"default_self_signed": false,
			},
			"http_options": map[string]any{
				"compress": true,
				"response": map[string]any{
					"headers": map[string]any{
						"fly-request-id": false,
						"fly-wasnt-here": "yes, it was",
						"multi-valued":   []any{"value1", "value2"},
					},
				},
			},
			"checks": []map[string]any{
				{
					"interval":        "1m21s",
					"timeout":         "7s",
					"grace_period":    "2s",
					"method":          "GET",
					"path":            "/",
					"protocol":        "https",
					"tls_skip_verify": true,
					"tls_server_name": "sni2.com",
					"headers": map[string]any{
						"My-Custom-Header": "whatever",
					},
				},
			},
		},

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
			"max_unavailable": 0.2,
		},
		"env": map[string]any{
			"FOO": "BAR",
		},
		"metrics": []map[string]any{
			{
				"port": int64(9999),
				"path": "/metrics",
			},
			{
				"port":      int64(9998),
				"path":      "/metrics",
				"processes": []any{"web"},
			},
		},
		"statics": []map[string]any{
			{
				"guest_path": "/path/to/statics",
				"url_prefix": "/static-assets",
			},
		},
		"files": []map[string]any{
			{
				"guest_path":  "/path/to/hello.txt",
				"raw_value":   "aGVsbG8gd29ybGQK",
				"local_path":  "",
				"secret_name": "",
			},
			{
				"guest_path":  "/path/to/secret.txt",
				"raw_value":   "",
				"secret_name": "SUPER_SECRET",
				"local_path":  "",
			},
			{
				"guest_path":  "/path/to/config.yaml",
				"raw_value":   "",
				"secret_name": "",
				"local_path":  "/local/path/config.yaml",
				"processes":   []any{"web"},
			},
		},
		"mounts": []map[string]any{{
			"source":      "data",
			"destination": "/data",
		}},
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
				"grace_period":    "27s",
				"method":          "GET",
				"path":            "/status",
				"protocol":        "https",
				"tls_skip_verify": true,
				"tls_server_name": "sni3.com",
				"headers": map[string]any{
					"Content-Type":  "application/json",
					"Authorization": "super-duper-secret",
				},
			},
		},
		"services": []map[string]any{
			{
				"internal_port":        int64(8081),
				"protocol":             "tcp",
				"processes":            []any{"app"},
				"auto_start_machines":  false,
				"auto_stop_machines":   false,
				"min_machines_running": int64(1),
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
						"interval":     "21s",
						"timeout":      "4s",
						"grace_period": "1s",
					},
				},
				"http_checks": []map[string]any{
					{
						"interval":        "1m21s",
						"timeout":         "7s",
						"grace_period":    "2s",
						"method":          "GET",
						"path":            "/",
						"protocol":        "https",
						"tls_skip_verify": true,
						"tls_server_name": "sni.com",
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

func TestFromDefinitionEnvAsList(t *testing.T) {
	cfg, err := cfgFromJSON(`{"env": [{"ONE": "one", "TWO": 2}, {"TRUE": true}]}`)
	require.NoError(t, err)

	want := map[string]string{
		"ONE":  "one",
		"TWO":  "2",
		"TRUE": "true",
	}
	assert.Equal(t, want, cfg.Env)
}

func TestFromDefinitionChecksAsList(t *testing.T) {
	cfg, err := cfgFromJSON(`{"checks": [{"name": "pg", "port": 80}]}`)
	require.NoError(t, err)

	want := map[string]*ToplevelCheck{
		"pg": {Port: api.Pointer(80)},
	}
	assert.Equal(t, want, cfg.Checks)
}

func TestFromDefinitionChecksAsEmptyList(t *testing.T) {
	cfg, err := cfgFromJSON(`{"checks": []}`)
	require.NoError(t, err)
	assert.Nil(t, cfg.Checks)
}

func TestFromDefinitionKillTimeoutInteger(t *testing.T) {
	cfg, err := cfgFromJSON(`{"kill_timeout": 20}`)
	require.NoError(t, err)
	assert.Equal(t, api.MustParseDuration("20s"), cfg.KillTimeout)
}

func TestFromDefinitionKillTimeoutFloat(t *testing.T) {
	cfg, err := cfgFromJSON(`{"kill_timeout": 1.5}`)
	require.NoError(t, err)
	assert.Equal(t, api.MustParseDuration("1s"), cfg.KillTimeout)
}

func TestFromDefinitionKillTimeoutString(t *testing.T) {
	cfg, err := cfgFromJSON(`{"kill_timeout": "10s"}`)
	require.NoError(t, err)
	assert.Equal(t, api.MustParseDuration("10s"), cfg.KillTimeout)
}

func dFromJSON(jsonBody string) (*api.Definition, error) {
	ret := &api.Definition{}
	err := json.Unmarshal([]byte(jsonBody), ret)
	return ret, err
}

func cfgFromJSON(jsonBody string) (*Config, error) {
	def, err := dFromJSON(jsonBody)
	if err != nil {
		return nil, err
	}
	return FromDefinition(def)
}
