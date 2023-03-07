package appconfig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
)

func TestLoadTOMLAppConfigWithAppName(t *testing.T) {
	const path = "./testdata/app-name.toml"

	p, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.AppName, "test-app")
}

func TestLoadTOMLAppConfigWithBuilderName(t *testing.T) {
	const path = "./testdata/build.toml"

	p, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Builder, "builder/name")
}

func TestLoadTOMLAppConfigWithImage(t *testing.T) {
	const path = "./testdata/image.toml"

	p, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Image, "image/name")
}

func TestLoadTOMLAppConfigWithDockerfile(t *testing.T) {
	const path = "./testdata/docker.toml"

	p, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Dockerfile, "./Dockerfile")
}

func TestLoadTOMLAppConfigWithBuilderNameAndArgs(t *testing.T) {
	const path = "./testdata/build-with-args.toml"

	p, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Args, map[string]string{"A": "B", "C": "D"})
}

func TestLoadTOMLAppConfigInvalidV2(t *testing.T) {
	const path = "./testdata/always-invalid-v2.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Error(t, cfg.v2UnmarshalError)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/always-invalid-v2.toml",
		v2UnmarshalError: fmt.Errorf("Unknown type for service concurrency: int64"),

		AppName: "unsupported-format",
		Build: &Build{
			Builder:           "dockerfile",
			Image:             "foo/fighter",
			Builtin:           "whatisthis",
			Dockerfile:        "Dockerfile",
			Ignorefile:        ".gitignore",
			DockerBuildTarget: "target",
			Buildpacks:        []string{"packme", "well"},
			Settings: map[string]any{
				"foo":   "bar",
				"other": int64(2),
			},

			Args: map[string]string{
				"param1": "value1",
				"param2": "value2",
			},
		},

		RawDefinition: map[string]any{
			"app": "unsupported-format",
			"build": map[string]any{
				"builder":      "dockerfile",
				"image":        "foo/fighter",
				"builtin":      "whatisthis",
				"dockerfile":   "Dockerfile",
				"ignorefile":   ".gitignore",
				"build-target": "target",
				"buildpacks":   []any{"packme", "well"},
				"args": map[string]any{
					"param1": "value1",
					"param2": "value2",
				},
				"settings": map[string]any{
					"foo":   "bar",
					"other": int64(2),
				},
			},
			"services": []map[string]any{{
				"concurrency":   int64(20),
				"internal_port": "8080",
			}},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigExperimental(t *testing.T) {
	const path = "./testdata/experimental-alt.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath: "./testdata/experimental-alt.toml",
		AppName:        "foo",
		Experimental: &Experimental{
			Cmd:        []string{"cmd"},
			Entrypoint: []string{"entrypoint"},
			Exec:       []string{"exec"},
		},
		RawDefinition: map[string]any{
			"app": "foo",
			"experimental": map[string]any{
				"cmd":        "cmd",
				"entrypoint": "entrypoint",
				"exec":       "exec",
			},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigMountsArray(t *testing.T) {
	const path = "./testdata/mounts-array.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath: "./testdata/mounts-array.toml",
		AppName:        "foo",
		Mounts: &Volume{
			Source:      "pg_data",
			Destination: "/data",
		},
		RawDefinition: map[string]any{
			"app": "foo",
			"mounts": []map[string]any{{
				"source":      "pg_data",
				"destination": "/data",
			}},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigOldFormat(t *testing.T) {
	const path = "./testdata/old-format.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath: "./testdata/old-format.toml",
		AppName:        "foo",
		Env: map[string]string{
			"FOO": "STRING",
			"BAR": "123",
		},
		Mounts: &Volume{
			Source:      "data",
			Destination: "/data",
		},
		Services: []Service{
			{
				InternalPort: 8080,
				Ports: []api.MachinePort{
					{
						Port:     api.Pointer(80),
						Handlers: []string{"http"},
					},
				},
				Concurrency: &api.MachineServiceConcurrency{
					Type:      "requests",
					HardLimit: 23,
					SoftLimit: 12,
				},
				TCPChecks: []*ServiceTCPCheck{
					{
						Interval: mustParseDuration("10s"),
						Timeout:  mustParseDuration("2s"),
					},
					{
						Interval: mustParseDuration("20s"),
						Timeout:  mustParseDuration("3s"),
					},
				},
				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval: mustParseDuration("30s"),
						Timeout:  mustParseDuration("4s"),
					},
					{
						Interval: mustParseDuration("20s"),
						Timeout:  mustParseDuration("3s"),
					},
				},
			},
		},
		RawDefinition: map[string]any{
			"app": "foo",
			"env": map[string]any{
				"FOO": "STRING",
				"BAR": int64(123),
			},
			"experimental": map[string]any{},
			"mount": map[string]any{
				"source":      "data",
				"destination": "/data",
			},
			"processes": []map[string]any{{}},
			"services": []map[string]any{{
				"internal_port": "8080",
				"ports": []map[string]any{
					{"port": "80", "handlers": []any{"http"}},
				},
				"concurrency": "12,23",
				"tcp_checks": []map[string]any{
					{"interval": int64(10000), "timeout": int64(2000)},
					{"interval": "20s", "timeout": "3s"},
				},
				"http_checks": []map[string]any{
					{"interval": int64(30000), "timeout": int64(4000)},
					{"interval": "20s", "timeout": "3s"},
				},
			}},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigReferenceFormat(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)

	// Nullify cfg.RawDefinition because it won't mutate per test in TestLoadTOMLAppConfigOldFormat
	cfg.RawDefinition = nil

	assert.Equal(t, &Config{
		configFilePath: "./testdata/full-reference.toml",
		AppName:        "foo",
		KillSignal:     "SIGTERM",
		KillTimeout:    3,
		PrimaryRegion:  "sea",
		Experimental: &Experimental{
			Cmd:          []string{"cmd"},
			Entrypoint:   []string{"entrypoint"},
			Exec:         []string{"exec"},
			AutoRollback: true,
			EnableConsul: true,
			EnableEtcd:   true,
		},

		Build: &Build{
			Builder:           "dockerfile",
			Image:             "foo/fighter",
			Builtin:           "whatisthis",
			Dockerfile:        "Dockerfile",
			Ignorefile:        ".gitignore",
			DockerBuildTarget: "target",
			Buildpacks:        []string{"packme", "well"},
			Settings: map[string]any{
				"foo":   "bar",
				"other": float64(2),
			},

			Args: map[string]string{
				"param1": "value1",
				"param2": "value2",
			},
		},

		Deploy: &Deploy{
			ReleaseCommand: "release command",
			Strategy:       "rolling-eyes",
		},

		Env: map[string]string{
			"FOO": "BAR",
		},

		Metrics: &api.MachineMetrics{
			Port: 9999,
			Path: "/metrics",
		},

		HttpService: &HTTPService{
			InternalPort: 8080,
			ForceHttps:   true,
			Concurrency: &api.MachineServiceConcurrency{
				Type:      "donuts",
				HardLimit: 10,
				SoftLimit: 4,
			},
		},

		Statics: []Static{
			{
				GuestPath: "/path/to/statics",
				UrlPrefix: "/static-assets",
			},
		},

		Mounts: &Volume{
			Source:      "data",
			Destination: "/data",
		},

		Processes: map[string]string{
			"web":  "run web",
			"task": "task all day",
		},

		Checks: map[string]*ToplevelCheck{
			"status": {
				Port:              api.Pointer(2020),
				Type:              api.Pointer("http"),
				Interval:          mustParseDuration("10s"),
				Timeout:           mustParseDuration("2s"),
				GracePeriod:       mustParseDuration("27s"),
				HTTPMethod:        api.Pointer("GET"),
				HTTPPath:          api.Pointer("/status"),
				HTTPProtocol:      api.Pointer("https"),
				HTTPTLSSkipVerify: api.Pointer(true),
				HTTPHeaders: map[string]string{
					"Content-Type":  "application/json",
					"Authorization": "super-duper-secret",
				},
			},
		},

		Services: []Service{
			{
				InternalPort: 8081,
				Protocol:     "tcp",
				Processes:    []string{"app"},

				Concurrency: &api.MachineServiceConcurrency{
					Type:      "requests",
					HardLimit: 22,
					SoftLimit: 13,
				},

				Ports: []api.MachinePort{
					{
						Port:       api.Pointer(80),
						StartPort:  api.Pointer(100),
						EndPort:    api.Pointer(200),
						Handlers:   []string{"https"},
						ForceHttps: true,
					},
				},

				TCPChecks: []*ServiceTCPCheck{
					{
						Interval:     mustParseDuration("21s"),
						Timeout:      mustParseDuration("4s"),
						GracePeriod:  mustParseDuration("1s"),
						RestartLimit: 3,
					},
				},

				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval:          mustParseDuration("81s"),
						Timeout:           mustParseDuration("7s"),
						GracePeriod:       mustParseDuration("2s"),
						RestartLimit:      4,
						HTTPMethod:        api.Pointer("GET"),
						HTTPPath:          api.Pointer("/"),
						HTTPProtocol:      api.Pointer("https"),
						HTTPTLSSkipVerify: api.Pointer(true),
						HTTPHeaders: map[string]string{
							"My-Custom-Header": "whatever",
						},
					},
					{
						Interval:   mustParseDuration("33s"),
						Timeout:    mustParseDuration("10s"),
						HTTPMethod: api.Pointer("POST"),
						HTTPPath:   api.Pointer("/check2"),
					},
				},
			},
		},
	}, cfg)
}

func mustParseDuration(v any) *api.Duration {
	d := &api.Duration{}
	if err := d.ParseDuration(v); err != nil {
		panic(err)
	}
	return d
}
