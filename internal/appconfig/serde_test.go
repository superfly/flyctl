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

func TestLoadTOMLAppConfigServicePorts(t *testing.T) {
	const path = "./testdata/services-ports.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	want := []Service{{
		Protocol:     "tcp",
		InternalPort: 8080,
		Ports: []api.MachinePort{
			{
				Port: api.Pointer(80),
				TLSOptions: &api.TLSOptions{
					ALPN:     []string{"h2", "http/1.1"},
					Versions: []string{"TLSv1.2", "TLSv1.3"},
				},
				HTTPOptions: &api.HTTPOptions{
					Compress: api.Pointer(true),
					Response: &api.HTTPResponseOptions{
						Headers: map[string]any{
							"fly-request-id": false,
							"fly-wasnt-here": "yes, it was",
							"multi-valued":   []any{"value1", "value2"},
						},
					},
				},
			},
			{
				Port:     api.Pointer(82),
				Handlers: []string{"proxy_proto"},
				ProxyProtoOptions: &api.ProxyProtoOptions{
					Version: "v2",
				},
			},
		},
	}}

	assert.Equal(t, want, p.Services)
}

func TestLoadTOMLAppConfigServiceMulti(t *testing.T) {
	const path = "./testdata/services-multi.toml"

	p, err := LoadConfig(path)
	require.NoError(t, err)
	want := []Service{
		{
			Protocol:     "tcp",
			InternalPort: 8081,
			Concurrency: &api.MachineServiceConcurrency{
				Type:      "requests",
				HardLimit: 22,
				SoftLimit: 13,
			},
		},
		{
			Protocol:     "tcp",
			InternalPort: 9999,
			Concurrency: &api.MachineServiceConcurrency{
				Type:      "connections",
				HardLimit: 10,
				SoftLimit: 8,
			},
		},
	}
	assert.Equal(t, want, p.Services)
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
		KillTimeout:      api.MustParseDuration("3s"),
		Metrics: []*Metrics{{
			MachineMetrics: &api.MachineMetrics{
				Path: "/foo",
				Port: 9000,
			},
		}},
		Experimental: &Experimental{
			Cmd:        []string{"cmd"},
			Entrypoint: []string{"entrypoint"},
			Exec:       []string{"exec"},
		},
		RawDefinition: map[string]any{
			"app": "foo",
			"experimental": map[string]any{
				"cmd":          "cmd",
				"entrypoint":   "entrypoint",
				"exec":         "exec",
				"kill_timeout": int64(3),
				"metrics_path": "/foo",
				"metrics_port": int64(9000),
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
		Mounts: []Mount{{
			Source:      "pg_data",
			Destination: "/data",
		}},
		RawDefinition: map[string]any{
			"app": "foo",
			"mounts": []map[string]any{{
				"source":      "pg_data",
				"destination": "/data",
			}},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigEnvList(t *testing.T) {
	const path = "./testdata/env-list.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	want := map[string]string{
		"FOO":  "BAR",
		"TWO":  "2",
		"TRUE": "true",
	}
	assert.Equal(t, want, cfg.Env)
}

func TestLoadTOMLAppConfigOldFormat(t *testing.T) {
	const path = "./testdata/old-format.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/old-format.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		Build: &Build{
			DockerBuildTarget: "thalayer",
		},
		Env: map[string]string{
			"FOO": "STRING",
			"BAR": "123",
		},
		Mounts: []Mount{{
			Source:      "data",
			Destination: "/data",
		}},
		Metrics: []*Metrics{{
			MachineMetrics: &api.MachineMetrics{
				Port: 9999,
				Path: "/metrics",
			},
		}},
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
						HTTPHeaders: map[string]string{
							"origin": "http://localhost:8000",
						},
					},
					{
						Interval: api.MustParseDuration("20s"),
						Timeout:  api.MustParseDuration("3s"),
						HTTPHeaders: map[string]string{
							"fly-healthcheck": "1",
							"metoo":           "true",
							"astring":         "string",
						},
					},
				},
			},
		},
		RawDefinition: map[string]any{
			"app": "foo",
			"build": map[string]any{
				"build_target": "thalayer",
			},
			"env": map[string]any{
				"FOO": "STRING",
				"BAR": int64(123),
			},
			"experimental": map[string]any{},
			"mount": map[string]any{
				"source":      "data",
				"destination": "/data",
			},
			"metrics": map[string]any{
				"port": int64(9999),
				"path": "/metrics",
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
					{
						"interval": int64(30000),
						"timeout":  int64(4000),
						"headers": []map[string]any{
							{"name": "origin", "value": "http://localhost:8000"},
						},
					},
					{
						"interval":     "20s",
						"timeout":      "3s",
						"grace_period": "",
						"headers":      map[string]any{"fly-healthcheck": int64(1), "astring": "string", "metoo": true},
					},
				},
			}},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigOldProcesses(t *testing.T) {
	const path = "./testdata/old-processes.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, &Config{
		configFilePath:   "./testdata/old-processes.toml",
		defaultGroupName: "app",
		Processes: map[string]string{
			"web":    "./web",
			"worker": "./worker",
		},
		RawDefinition: map[string]any{
			"processes": []map[string]any{
				{
					"name":    "web",
					"command": "./web",
				},
				{
					"name":    "worker",
					"command": "./worker",
				},
			},
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
		KillTimeout:      api.MustParseDuration("3s"),
		SwapSizeMB:       api.Pointer(512),
		PrimaryRegion:    "sea",
		ConsoleCommand:   "/bin/bash",
		HostDedicationID: "06031957",
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
			MaxUnavailable: api.Pointer(0.2),
		},

		Env: map[string]string{
			"FOO": "BAR",
		},

		Metrics: []*Metrics{
			{
				MachineMetrics: &api.MachineMetrics{
					Port: 9999,
					Path: "/metrics",
				},
			},
			{
				MachineMetrics: &api.MachineMetrics{
					Port: 9998,
					Path: "/metrics",
				},
				Processes: []string{"web"},
			},
		},

		HTTPService: &HTTPService{
			InternalPort:       8080,
			ForceHTTPS:         true,
			AutoStartMachines:  api.Pointer(false),
			AutoStopMachines:   api.Pointer(false),
			MinMachinesRunning: api.Pointer(0),
			Concurrency: &api.MachineServiceConcurrency{
				Type:      "donuts",
				HardLimit: 10,
				SoftLimit: 4,
			},
			TLSOptions: &api.TLSOptions{
				ALPN:              []string{"h2", "http/1.1"},
				Versions:          []string{"TLSv1.2", "TLSv1.3"},
				DefaultSelfSigned: api.Pointer(false),
			},
			HTTPOptions: &api.HTTPOptions{
				Compress: api.Pointer(true),
				Response: &api.HTTPResponseOptions{
					Headers: map[string]any{
						"fly-request-id": false,
						"fly-wasnt-here": "yes, it was",
						"multi-valued":   []any{"value1", "value2"},
					},
				},
			},
			HTTPChecks: []*ServiceHTTPCheck{
				{
					Interval:          api.MustParseDuration("81s"),
					Timeout:           api.MustParseDuration("7s"),
					GracePeriod:       api.MustParseDuration("2s"),
					HTTPMethod:        api.Pointer("GET"),
					HTTPPath:          api.Pointer("/"),
					HTTPProtocol:      api.Pointer("https"),
					HTTPTLSSkipVerify: api.Pointer(true),
					HTTPTLSServerName: api.Pointer("sni2.com"),
					HTTPHeaders: map[string]string{
						"My-Custom-Header": "whatever",
					},
				},
			},
		},

		Statics: []Static{
			{
				GuestPath: "/path/to/statics",
				UrlPrefix: "/static-assets",
			},
		},

		Files: []File{
			{
				GuestPath: "/path/to/hello.txt",
				RawValue:  "aGVsbG8gd29ybGQK",
			},
			{
				GuestPath:  "/path/to/secret.txt",
				SecretName: "SUPER_SECRET",
			},
			{
				GuestPath: "/path/to/config.yaml",
				LocalPath: "/local/path/config.yaml",
				Processes: []string{"web"},
			},
		},

		Mounts: []Mount{{
			Source:      "data",
			Destination: "/data",
		}},

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
				HTTPTLSServerName: api.Pointer("sni3.com"),
				HTTPHeaders: map[string]string{
					"Content-Type":  "application/json",
					"Authorization": "super-duper-secret",
				},
			},
		},

		Services: []Service{
			{
				InternalPort:       8081,
				Protocol:           "tcp",
				Processes:          []string{"app"},
				AutoStartMachines:  api.Pointer(false),
				AutoStopMachines:   api.Pointer(false),
				MinMachinesRunning: api.Pointer(1),

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
						ForceHTTPS: true,
					},
				},

				TCPChecks: []*ServiceTCPCheck{
					{
						Interval:    api.MustParseDuration("21s"),
						Timeout:     api.MustParseDuration("4s"),
						GracePeriod: api.MustParseDuration("1s"),
					},
				},

				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval:          api.MustParseDuration("81s"),
						Timeout:           api.MustParseDuration("7s"),
						GracePeriod:       api.MustParseDuration("2s"),
						HTTPMethod:        api.Pointer("GET"),
						HTTPPath:          api.Pointer("/"),
						HTTPProtocol:      api.Pointer("https"),
						HTTPTLSSkipVerify: api.Pointer(true),
						HTTPTLSServerName: api.Pointer("sni.com"),
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
