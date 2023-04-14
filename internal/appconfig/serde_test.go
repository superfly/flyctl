package appconfig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
)

func TestLoadTOMLAppConfigWithAppName(t *testing.T) {
	const path = "./testdata/app-name.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, p.AppName, "test-app")
}

func TestLoadTOMLAppConfigWithBuilderName(t *testing.T) {
	const path = "./testdata/build.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, p.Build.Builder, "builder/name")
}

func TestLoadTOMLAppConfigWithImage(t *testing.T) {
	const path = "./testdata/image.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, p.Build.Image, "image/name")
}

func TestLoadTOMLAppConfigWithDockerfile(t *testing.T) {
	const path = "./testdata/docker.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, p.Build.Dockerfile, "./Dockerfile")
}

func TestLoadTOMLAppConfigWithBuilderNameAndArgs(t *testing.T) {
	const path = "./testdata/build-with-args.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, p.Build.Args, map[string]string{"A": "B", "C": "D"})
}

func TestLoadTOMLAppConfigWithEmptyService(t *testing.T) {
	const path = "./testdata/services-emptysection.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Nil(t, p.Services)
}

func TestLoadTOMLAppConfigInvalidV2(t *testing.T) {
	const path = "./testdata/always-invalid-v2.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
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
	require.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/experimental-alt.toml",
		defaultGroupName: "app",
		AppName:          "foo",
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
	require.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/mounts-array.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		Mounts: &Mount{
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
	require.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/old-format.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		Env: map[string]string{
			"FOO": "STRING",
			"BAR": "123",
		},
		Mounts: &Mount{
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
						Interval: api.MustParseDuration("10s"),
						Timeout:  api.MustParseDuration("2s"),
					},
					{
						Interval: api.MustParseDuration("20s"),
						Timeout:  api.MustParseDuration("3s"),
					},
				},
				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval: api.MustParseDuration("30s"),
						Timeout:  api.MustParseDuration("4s"),
					},
					{
						Interval: api.MustParseDuration("20s"),
						Timeout:  api.MustParseDuration("3s"),
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

func TestLoadTOMLAppConfigOldChecksFormat(t *testing.T) {
	const path = "./testdata/old-pg-checks.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/old-pg-checks.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		Checks: map[string]*ToplevelCheck{
			"pg": {
				Port:     api.Pointer(5500),
				Type:     api.Pointer("http"),
				HTTPPath: api.Pointer("/flycheck/pg"),
			},
		},
		RawDefinition: map[string]any{
			"app": "foo",
			"checks": map[string]any{
				"pg": map[string]any{
					"type":    "http",
					"port":    int64(5500),
					"path":    "/flycheck/pg",
					"headers": []any{},
				},
			},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigReferenceFormat(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	// Nullify cfg.RawDefinition because it won't mutate per test in TestLoadTOMLAppConfigOldFormat
	cfg.RawDefinition = nil

	assert.Equal(t, &Config{
		configFilePath:   "./testdata/full-reference.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		KillSignal:       api.Pointer("SIGTERM"),
		KillTimeout:      api.Pointer(3),
		PrimaryRegion:    "sea",
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

		HTTPService: &HTTPService{
			InternalPort: 8080,
			ForceHTTPS:   true,
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

		Mounts: &Mount{
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
				Interval:          api.MustParseDuration("10s"),
				Timeout:           api.MustParseDuration("2s"),
				GracePeriod:       api.MustParseDuration("27s"),
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
						Interval:     api.MustParseDuration("21s"),
						Timeout:      api.MustParseDuration("4s"),
						GracePeriod:  api.MustParseDuration("1s"),
						RestartLimit: 3,
					},
				},

				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval:          api.MustParseDuration("81s"),
						Timeout:           api.MustParseDuration("7s"),
						GracePeriod:       api.MustParseDuration("2s"),
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
						Interval:   api.MustParseDuration("33s"),
						Timeout:    api.MustParseDuration("10s"),
						HTTPMethod: api.Pointer("POST"),
						HTTPPath:   api.Pointer("/check2"),
					},
				},
			},
		},
	}, cfg)
}
