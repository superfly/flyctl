package appconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
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
		Ports: []fly.MachinePort{
			{
				Port: fly.Pointer(80),
				TLSOptions: &fly.TLSOptions{
					ALPN:     []string{"h2", "http/1.1"},
					Versions: []string{"TLSv1.2", "TLSv1.3"},
				},
				HTTPOptions: &fly.HTTPOptions{
					Compress: fly.Pointer(true),
					Response: &fly.HTTPResponseOptions{
						Headers: map[string]any{
							"fly-request-id": false,
							"fly-wasnt-here": "yes, it was",
							"multi-valued":   []any{"value1", "value2"},
						},
					},
				},
			},
			{
				Port:     fly.Pointer(82),
				Handlers: []string{"proxy_proto"},
				ProxyProtoOptions: &fly.ProxyProtoOptions{
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
			Concurrency: &fly.MachineServiceConcurrency{
				Type:      "requests",
				HardLimit: 22,
				SoftLimit: 13,
			},
		},
		{
			Protocol:     "tcp",
			InternalPort: 9999,
			Concurrency: &fly.MachineServiceConcurrency{
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
		AppName:          "unsupported-format",
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
		KillTimeout:      fly.MustParseDuration("3s"),
		Metrics: []*Metrics{{
			MachineMetrics: &fly.MachineMetrics{
				Path: "/foo",
				Port: 9000,
			},
		}},
		Experimental: &Experimental{
			Cmd:        []string{"cmd"},
			Entrypoint: []string{"entrypoint"},
			Exec:       []string{"exec"},
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
	}, cfg)
}

func TestLoadTOMLAppConfigFormatQuirks(t *testing.T) {
	const path = "./testdata/format-quirks.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, &Config{
		configFilePath:   "./testdata/format-quirks.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		Compute: []*Compute{{
			Memory: "512",
		}},
		Mounts: []Mount{{
			Source:            "data",
			Destination:       "/data",
			InitialSize:       "200",
			SnapshotRetention: fly.Pointer(10),
		}},
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
			MachineMetrics: &fly.MachineMetrics{
				Port: 9999,
				Path: "/metrics",
			},
		}},
		Services: []Service{
			{
				InternalPort:     8080,
				AutoStopMachines: fly.Pointer(fly.MachineAutostopOff),
				Ports: []fly.MachinePort{
					{
						Port:     fly.Pointer(80),
						Handlers: []string{"http"},
					},
				},
				Concurrency: &fly.MachineServiceConcurrency{
					Type:      "requests",
					HardLimit: 23,
					SoftLimit: 12,
				},
				TCPChecks: []*ServiceTCPCheck{
					{
						Interval: fly.MustParseDuration("10s"),
						Timeout:  fly.MustParseDuration("2s"),
					},
					{
						Interval: fly.MustParseDuration("20s"),
						Timeout:  fly.MustParseDuration("3s"),
					},
				},
				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval: fly.MustParseDuration("30s"),
						Timeout:  fly.MustParseDuration("4s"),
						HTTPHeaders: map[string]string{
							"origin": "http://localhost:8000",
						},
					},
					{
						Interval: fly.MustParseDuration("20s"),
						Timeout:  fly.MustParseDuration("3s"),
						HTTPHeaders: map[string]string{
							"fly-healthcheck": "1",
							"metoo":           "true",
							"astring":         "string",
						},
					},
				},
			},
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
				Port:     fly.Pointer(5500),
				Type:     fly.Pointer("http"),
				HTTPPath: fly.Pointer("/flycheck/pg"),
			},
		},
	}, cfg)
}

func TestLoadTOMLAppConfigReferenceFormat(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, &Config{
		configFilePath:   "./testdata/full-reference.toml",
		defaultGroupName: "app",
		AppName:          "foo",
		KillSignal:       fly.Pointer("SIGTERM"),
		KillTimeout:      fly.MustParseDuration("3s"),
		SwapSizeMB:       fly.Pointer(512),
		PrimaryRegion:    "sea",
		ConsoleCommand:   "/bin/bash",
		HostDedicationID: "06031957",
		Compute: []*Compute{
			{
				Size:   "shared-cpu-1x",
				Memory: "8gb",
				MachineGuest: &fly.MachineGuest{
					CPUKind:          "performance",
					CPUs:             8,
					MemoryMB:         8192,
					GPUs:             2,
					GPUKind:          "a100-pcie-40gb",
					HostDedicationID: "isolated-xxx",
					KernelArgs:       []string{"quiet"},
				},
				Processes: []string{"app"},
			},
			{
				MachineGuest: &fly.MachineGuest{
					MemoryMB: 4096,
				},
			},
		},
		Experimental: &Experimental{
			Cmd:          []string{"cmd"},
			Entrypoint:   []string{"entrypoint"},
			Exec:         []string{"exec"},
			AutoRollback: true,
			EnableConsul: true,
			EnableEtcd:   true,
		},

		Restart: []Restart{
			{
				Policy:     "always",
				MaxRetries: 3,
				Processes:  []string{"web"},
			},
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
			Strategy:              "rolling-eyes",
			MaxUnavailable:        fly.Pointer(0.2),
			ReleaseCommand:        "release command",
			ReleaseCommandTimeout: fly.MustParseDuration("3m"),
			ReleaseCommandCompute: &Compute{
				Size:   "performance-2x",
				Memory: "8g",
			},
		},

		Env: map[string]string{
			"FOO": "BAR",
		},

		Metrics: []*Metrics{
			{
				MachineMetrics: &fly.MachineMetrics{
					Port: 9999,
					Path: "/metrics",
				},
			},
			{
				MachineMetrics: &fly.MachineMetrics{
					Port: 9998,
					Path: "/metrics",
				},
				Processes: []string{"web"},
			},
		},

		HTTPService: &HTTPService{
			InternalPort:       8080,
			ForceHTTPS:         true,
			AutoStartMachines:  fly.Pointer(false),
			AutoStopMachines:   fly.Pointer(fly.MachineAutostopOff),
			MinMachinesRunning: fly.Pointer(0),
			Concurrency: &fly.MachineServiceConcurrency{
				Type:      "donuts",
				HardLimit: 10,
				SoftLimit: 4,
			},
			TLSOptions: &fly.TLSOptions{
				ALPN:              []string{"h2", "http/1.1"},
				Versions:          []string{"TLSv1.2", "TLSv1.3"},
				DefaultSelfSigned: fly.Pointer(false),
			},
			HTTPOptions: &fly.HTTPOptions{
				Compress:    fly.Pointer(true),
				IdleTimeout: UintPointer(600),
				Response: &fly.HTTPResponseOptions{
					Headers: map[string]any{
						"fly-request-id": false,
						"fly-wasnt-here": "yes, it was",
						"multi-valued":   []any{"value1", "value2"},
					},
				},
			},
			HTTPChecks: []*ServiceHTTPCheck{
				{
					Interval:          fly.MustParseDuration("81s"),
					Timeout:           fly.MustParseDuration("7s"),
					GracePeriod:       fly.MustParseDuration("2s"),
					HTTPMethod:        fly.Pointer("GET"),
					HTTPPath:          fly.Pointer("/"),
					HTTPProtocol:      fly.Pointer("https"),
					HTTPTLSSkipVerify: fly.Pointer(true),
					HTTPTLSServerName: fly.Pointer("sni2.com"),
					HTTPHeaders: map[string]string{
						"My-Custom-Header": "whatever",
					},
				},
			},
			MachineChecks: []*ServiceMachineCheck{
				{
					Command:     []string{"curl", "https://fly.io"},
					Entrypoint:  []string{"/bin/sh"},
					Image:       "curlimages/curl",
					KillSignal:  fly.StringPointer("SIGKILL"),
					KillTimeout: fly.MustParseDuration("5s"),
				},
			},
		},

		MachineChecks: []*ServiceMachineCheck{
			{
				Command:     []string{"curl", "https://fly.io"},
				Entrypoint:  []string{"/bin/sh"},
				Image:       "curlimages/curl",
				KillSignal:  fly.StringPointer("SIGKILL"),
				KillTimeout: fly.MustParseDuration("5s"),
			},
		},

		Statics: []Static{
			{
				GuestPath:     "/path/to/statics",
				UrlPrefix:     "/static-assets",
				TigrisBucket:  "example-bucket",
				IndexDocument: "index.html",
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
			Source:            "data",
			Destination:       "/data",
			InitialSize:       "30gb",
			SnapshotRetention: fly.Pointer(17),
		}},

		Processes: map[string]string{
			"web":  "run web",
			"task": "task all day",
		},

		Checks: map[string]*ToplevelCheck{
			"status": {
				Port:              fly.Pointer(2020),
				Type:              fly.Pointer("http"),
				Interval:          fly.MustParseDuration("10s"),
				Timeout:           fly.MustParseDuration("2s"),
				GracePeriod:       fly.MustParseDuration("27s"),
				HTTPMethod:        fly.Pointer("GET"),
				HTTPPath:          fly.Pointer("/status"),
				HTTPProtocol:      fly.Pointer("https"),
				HTTPTLSSkipVerify: fly.Pointer(true),
				HTTPTLSServerName: fly.Pointer("sni3.com"),
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
				AutoStartMachines:  fly.Pointer(false),
				AutoStopMachines:   fly.Pointer(fly.MachineAutostopOff),
				MinMachinesRunning: fly.Pointer(1),

				Concurrency: &fly.MachineServiceConcurrency{
					Type:      "requests",
					HardLimit: 22,
					SoftLimit: 13,
				},

				Ports: []fly.MachinePort{
					{
						Port:       fly.Pointer(80),
						StartPort:  fly.Pointer(100),
						EndPort:    fly.Pointer(200),
						Handlers:   []string{"https"},
						ForceHTTPS: true,
						HTTPOptions: &fly.HTTPOptions{
							IdleTimeout: UintPointer(600),
						},
					},
				},

				TCPChecks: []*ServiceTCPCheck{
					{
						Interval:    fly.MustParseDuration("21s"),
						Timeout:     fly.MustParseDuration("4s"),
						GracePeriod: fly.MustParseDuration("1s"),
					},
				},

				HTTPChecks: []*ServiceHTTPCheck{
					{
						Interval:          fly.MustParseDuration("81s"),
						Timeout:           fly.MustParseDuration("7s"),
						GracePeriod:       fly.MustParseDuration("2s"),
						HTTPMethod:        fly.Pointer("GET"),
						HTTPPath:          fly.Pointer("/"),
						HTTPProtocol:      fly.Pointer("https"),
						HTTPTLSSkipVerify: fly.Pointer(true),
						HTTPTLSServerName: fly.Pointer("sni.com"),
						HTTPHeaders: map[string]string{
							"My-Custom-Header": "whatever",
						},
					},
					{
						Interval:   fly.MustParseDuration("33s"),
						Timeout:    fly.MustParseDuration("10s"),
						HTTPMethod: fly.Pointer("POST"),
						HTTPPath:   fly.Pointer("/check2"),
					},
				},
				MachineChecks: []*ServiceMachineCheck{
					{
						Command:     []string{"curl", "https://fly.io"},
						Entrypoint:  []string{"/bin/sh"},
						Image:       "curlimages/curl",
						KillSignal:  fly.StringPointer("SIGKILL"),
						KillTimeout: fly.MustParseDuration("5s"),
					},
				},
			},
		},
	}, cfg)
}

func TestIsSameTOMLAppConfigReferenceFormat(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())

	flyToml := filepath.Join(t.TempDir(), "fly.toml")
	cfg.WriteToFile(flyToml)

	actual, err := LoadConfig(flyToml)
	require.NoError(t, err)
	require.NoError(t, actual.SetMachinesPlatform())

	cfg.configFilePath = ""
	actual.configFilePath = ""
	require.Equal(t, cfg, actual)
}

func TestIsSameJSONAppConfigReferenceFormat(t *testing.T) {
	const TOMLpath = "./testdata/full-reference.toml"
	TOMLcfg, err := LoadConfig(TOMLpath)
	require.NoError(t, err)

	JSONpath := filepath.Join(t.TempDir(), "full-reference.json")
	err = TOMLcfg.WriteToFile(JSONpath)
	require.NoError(t, err)

	JSONcfg, err := LoadConfig(JSONpath)
	require.NoError(t, err)

	TOMLcfg.configFilePath = ""
	JSONcfg.configFilePath = ""
	require.Equal(t, TOMLcfg, JSONcfg)
}

func TestIsSameYAMLAppConfigReferenceFormat(t *testing.T) {
	const TOMLpath = "./testdata/full-reference.toml"
	TOMLcfg, err := LoadConfig(TOMLpath)
	require.NoError(t, err)

	YAMLpath := filepath.Join(t.TempDir(), "full-reference.yaml")
	err = TOMLcfg.WriteToFile(YAMLpath)
	require.NoError(t, err)

	YAMLcfg, err := LoadConfig(YAMLpath)
	require.NoError(t, err)

	TOMLcfg.configFilePath = ""
	YAMLcfg.configFilePath = ""
	require.Equal(t, TOMLcfg, YAMLcfg)
}

func TestJSONPrettyPrint(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	JSONpath := filepath.Join(t.TempDir(), "full-reference.json")
	err = cfg.WriteToFile(JSONpath)
	require.NoError(t, err)

	buf, err := os.ReadFile(JSONpath)
	require.NoError(t, err)

	assert.Contains(t, string(buf), "{\n  \"app\": \"foo\",\n")
	assert.Contains(t, string(buf), ",\n\n  \"experimental\": {\n    \"cmd\": [\n")
	assert.Contains(t, string(buf), ",\n\n      \"processes\": [\n        \"web\"\n      ]\n    }\n  ]\n}\n")
}

func TestYAMLPrettyPrint(t *testing.T) {
	const path = "./testdata/full-reference.toml"
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	YAMLpath := filepath.Join(t.TempDir(), "full-reference.yaml")
	err = cfg.WriteToFile(YAMLpath)
	require.NoError(t, err)

	buf, err := os.ReadFile(YAMLpath)
	require.NoError(t, err)

	assert.Contains(t, string(buf), "\napp: foo\n")
	assert.Contains(t, string(buf), "\n\nexperimental:\n  cmd:\n    - cmd\n")
	assert.Contains(t, string(buf), "\n    processes:\n      - web\n")
}

func UintPointer(v uint32) *uint32 {
	return &v
}
