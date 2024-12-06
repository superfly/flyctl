package appconfig

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
)

// Usual Config response for fly.GetConfig GQL call
var GetConfigJSON = []byte(`
{
  "env": {},
  "experimental": {
    "auto_rollback": true
  },
  "kill_signal": "SIGINT",
  "kill_timeout": 5,
  "processes": [],
  "restart" : [
	{
		"policy": "always",
		"retries": 3,
		"processes": [
			"app"
		]
	}
  ],
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
	definition := &fly.Definition{}
	err := json.Unmarshal(GetConfigJSON, definition)
	assert.NoError(t, err)

	cfg, err := FromDefinition(definition)
	assert.NoError(t, err)

	assert.Equal(t, &Config{
		KillSignal:  fly.Pointer("SIGINT"),
		KillTimeout: fly.MustParseDuration("5s"),
		Restart: []Restart{
			{
				Policy:     RestartPolicyAlways,
				MaxRetries: 3,
				Processes:  []string{"app"},
			},
		},
		Experimental: &Experimental{
			AutoRollback: true,
		},
		Env: map[string]string{},
		Services: []Service{
			{
				InternalPort: 8080,
				Protocol:     "tcp",
				Concurrency: &fly.MachineServiceConcurrency{
					Type:      "connections",
					HardLimit: 25,
					SoftLimit: 20,
				},
				Ports: []fly.MachinePort{
					{
						Port:       fly.Pointer(80),
						Handlers:   []string{"http"},
						ForceHTTPS: true,
					},
					{
						Port:     fly.Pointer(443),
						Handlers: []string{"tls", "http"},
					},
				},
				Processes: []string{"app"},
				TCPChecks: []*ServiceTCPCheck{
					{
						Timeout:     fly.MustParseDuration("2s"),
						Interval:    fly.MustParseDuration("15s"),
						GracePeriod: fly.MustParseDuration("1s"),
					},
				},
			},
		},
		configFilePath:   "--config path unset--",
		defaultGroupName: "app",
	}, cfg)
}

func TestToDefinition(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)

	definition, err := cfg.ToDefinition()
	assert.NoError(t, err)
	assert.Equal(t, &fly.Definition{
		"app":                "foo",
		"primary_region":     "sea",
		"kill_signal":        "SIGTERM",
		"kill_timeout":       "3s",
		"swap_size_mb":       int64(512),
		"console_command":    "/bin/bash",
		"host_dedication_id": "06031957",
		"vm": []any{
			map[string]any{
				"size":               "shared-cpu-1x",
				"memory":             "8gb",
				"cpu_kind":           "performance",
				"cpus":               int64(8),
				"gpus":               int64(2),
				"gpu_kind":           "a100-pcie-40gb",
				"host_dedication_id": "isolated-xxx",
				"memory_mb":          int64(8192),
				"kernel_args":        []any{"quiet"},
				"processes":          []any{"app"},
			},
			map[string]any{
				"memory_mb": int64(4096),
			},
		},
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

		"restart": []any{
			map[string]any{
				"policy":    "always",
				"retries":   int64(3),
				"processes": []any{"web"},
			},
		},

		"http_service": map[string]any{
			"internal_port":        int64(8080),
			"force_https":          true,
			"auto_start_machines":  false,
			"auto_stop_machines":   "off",
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
				"compress":     true,
				"idle_timeout": int64(600),
				"response": map[string]any{
					"headers": map[string]any{
						"fly-request-id": false,
						"fly-wasnt-here": "yes, it was",
						"multi-valued":   []any{"value1", "value2"},
					},
				},
			},
			"checks": []any{
				map[string]any{
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
			"machine_checks": []any{
				map[string]any{
					"command":      []any{"curl", "https://fly.io"},
					"image":        "curlimages/curl",
					"entrypoint":   []any{"/bin/sh"},
					"kill_signal":  "SIGKILL",
					"kill_timeout": "5s",
				},
			},
		},
		"machine_checks": []any{
			map[string]any{
				"command":      []any{"curl", "https://fly.io"},
				"image":        "curlimages/curl",
				"entrypoint":   []any{"/bin/sh"},
				"kill_signal":  "SIGKILL",
				"kill_timeout": "5s",
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
			"strategy":                "rolling-eyes",
			"max_unavailable":         0.2,
			"release_command":         "release command",
			"release_command_timeout": "3m0s",
			"release_command_vm": map[string]any{
				"size":   "performance-2x",
				"memory": "8g",
			},
		},
		"env": map[string]any{
			"FOO": "BAR",
		},
		"metrics": []any{
			map[string]any{
				"port":  int64(9999),
				"path":  "/metrics",
				"https": false,
			},
			map[string]any{
				"port":      int64(9998),
				"path":      "/metrics",
				"processes": []any{"web"},
				"https":     false,
			},
		},
		"statics": []any{
			map[string]any{
				"guest_path":     "/path/to/statics",
				"url_prefix":     "/static-assets",
				"tigris_bucket":  "example-bucket",
				"index_document": "index.html",
			},
		},
		"files": []any{
			map[string]any{
				"guest_path": "/path/to/hello.txt",
				"raw_value":  "aGVsbG8gd29ybGQK",
			},
			map[string]any{
				"guest_path":  "/path/to/secret.txt",
				"secret_name": "SUPER_SECRET",
			},
			map[string]any{
				"guest_path": "/path/to/config.yaml",
				"local_path": "/local/path/config.yaml",
				"processes":  []any{"web"},
			},
		},
		"mounts": []any{map[string]any{
			"source":             "data",
			"destination":        "/data",
			"initial_size":       "30gb",
			"snapshot_retention": int64(17),
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
		"services": []any{
			map[string]any{
				"internal_port":        int64(8081),
				"protocol":             "tcp",
				"processes":            []any{"app"},
				"auto_start_machines":  false,
				"auto_stop_machines":   "off",
				"min_machines_running": int64(1),
				"concurrency": map[string]any{
					"type":       "requests",
					"hard_limit": int64(22),
					"soft_limit": int64(13),
				},
				"ports": []any{
					map[string]any{
						"port":       int64(80),
						"start_port": int64(100),
						"end_port":   int64(200),
						"handlers":   []any{"https"},
						"http_options": map[string]any{
							"idle_timeout": int64(600),
						},
						"force_https": true,
					},
				},
				"tcp_checks": []any{
					map[string]any{
						"interval":     "21s",
						"timeout":      "4s",
						"grace_period": "1s",
					},
				},
				"http_checks": []any{
					map[string]any{
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
					map[string]any{
						"interval": "33s",
						"timeout":  "10s",
						"method":   "POST",
						"path":     "/check2",
					},
				},
				"machine_checks": []any{
					map[string]any{
						"command":      []any{"curl", "https://fly.io"},
						"image":        "curlimages/curl",
						"entrypoint":   []any{"/bin/sh"},
						"kill_signal":  "SIGKILL",
						"kill_timeout": "5s",
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
		"pg": {Port: fly.Pointer(80)},
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
	assert.Equal(t, fly.MustParseDuration("20s"), cfg.KillTimeout)
}

func TestFromDefinitionKillTimeoutFloat(t *testing.T) {
	cfg, err := cfgFromJSON(`{"kill_timeout": 1.5}`)
	require.NoError(t, err)
	assert.Equal(t, fly.MustParseDuration("1s"), cfg.KillTimeout)
}

func TestFromDefinitionKillTimeoutString(t *testing.T) {
	cfg, err := cfgFromJSON(`{"kill_timeout": "10s"}`)
	require.NoError(t, err)
	assert.Equal(t, fly.MustParseDuration("10s"), cfg.KillTimeout)
}

func dFromJSON(jsonBody string) (*fly.Definition, error) {
	ret := &fly.Definition{}
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
