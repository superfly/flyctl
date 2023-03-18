package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
)

func TestSettersWithService(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-service.toml")
	assert.NoError(t, err)

	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status")
	cfg.SetConcurrency(12, 34)

	assert.Equal(t, &Config{
		configFilePath: "./testdata/setters-service.toml",
		AppName:        "setters",
		Services: []Service{{
			InternalPort: 1234,
			Protocol:     "tcp",
			Concurrency: &api.MachineServiceConcurrency{
				Type:      "connections",
				HardLimit: 34,
				SoftLimit: 12,
			},
			HTTPChecks: []*ServiceHTTPCheck{{
				Interval:          mustParseDuration("10s"),
				Timeout:           mustParseDuration("2s"),
				GracePeriod:       mustParseDuration("5s"),
				HTTPMethod:        api.Pointer("GET"),
				HTTPPath:          api.Pointer("/status"),
				HTTPProtocol:      api.Pointer("http"),
				HTTPTLSSkipVerify: api.Pointer(false),
			}},
		}},
		RawDefinition: map[string]any{
			"app": "setters",
			"services": []map[string]any{{
				"internal_port": 1234,
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"hard_limit": 34,
					"soft_limit": 12,
				},
				"http_checks": []map[string]any{{
					"interval":        10000,
					"grace_period":    "5s",
					"method":          "get",
					"path":            "/status",
					"protocol":        "http",
					"restart_limit":   0,
					"timeout":         2000,
					"tls_skip_verify": false,
				}},
			}},
		},
	}, cfg)
}

func TestSettersWithouServices(t *testing.T) {
	cfg := NewConfig()
	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status")
	cfg.SetConcurrency(12, 34)
	assert.Equal(t, &Config{RawDefinition: map[string]any{}}, cfg)
}

func TestSetEnvVariable(t *testing.T) {
	cfg := NewConfig()
	cfg.SetEnvVariable("a", "1")
	cfg.SetEnvVariables(map[string]string{"b": "2", "c": "3"})
	assert.Equal(t, &Config{
		Env: map[string]string{
			"a": "1",
			"b": "2",
			"c": "3",
		},
		RawDefinition: map[string]any{
			"env": map[string]string{
				"a": "1",
				"b": "2",
				"c": "3",
			},
		},
	}, cfg)
}

func TestReleaseCommand(t *testing.T) {
	cfg := NewConfig()
	cfg.SetReleaseCommand("/bin/app/run migrate")
	assert.Equal(t, &Config{
		Deploy: &Deploy{
			ReleaseCommand: "/bin/app/run migrate",
		},
		RawDefinition: map[string]any{
			"deploy": map[string]string{
				"release_command": "/bin/app/run migrate",
			},
		},
	}, cfg)
}

func TestReleaseCommand_DeploySectionExists(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-deploy.toml")
	assert.NoError(t, err)

	cfg.SetReleaseCommand("/bin/app/run migrate")
	assert.Equal(t, &Config{
		configFilePath: "./testdata/setters-deploy.toml",
		AppName:        "setters",
		Deploy: &Deploy{
			ReleaseCommand: "/bin/app/run migrate",
			Strategy:       "immediate",
		},
		RawDefinition: map[string]any{
			"app": "setters",
			"deploy": map[string]any{
				"release_command": "/bin/app/run migrate",
				"strategy":        "immediate",
			},
		},
	}, cfg)
}

func TestSetDocker(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, &Config{
		Experimental: &Experimental{
			Entrypoint: []string{"/entry"},
			Cmd:        []string{"/cmd"},
		},
		RawDefinition: map[string]any{
			"experimental": map[string]string{
				"entrypoint": "/entry",
				"cmd":        "/cmd",
			},
		},
	}, cfg)
}

func TestSetDocker_ExperimentalSectionExists(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-experimental.toml")
	assert.NoError(t, err)

	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, &Config{
		configFilePath: "./testdata/setters-experimental.toml",
		AppName:        "setters",
		Experimental: &Experimental{
			Entrypoint: []string{"/entry"},
			Cmd:        []string{"/cmd"},
			Exec:       []string{"/exec"},
		},
		RawDefinition: map[string]any{
			"app": "setters",
			"experimental": map[string]any{
				"entrypoint": "/entry",
				"cmd":        "/cmd",
				"exec":       "/exec",
			},
		},
	}, cfg)
}

func TestSetProcesses(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, &Config{
		Experimental: &Experimental{
			Entrypoint: []string{"/entry"},
			Cmd:        []string{"/cmd"},
		},
		RawDefinition: map[string]any{
			"experimental": map[string]string{
				"entrypoint": "/entry",
				"cmd":        "/cmd",
			},
		},
	}, cfg)
}

func TestSetProcesses_SectionExists(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-processes.toml")
	assert.NoError(t, err)

	cfg.SetProcess("app", "run-web")
	cfg.SetProcess("back", "run-back")
	assert.Equal(t, &Config{
		configFilePath: "./testdata/setters-processes.toml",
		AppName:        "setters",
		Processes: map[string]string{
			"app":  "run-web",
			"back": "run-back",
			"foo":  "bar",
		},
		RawDefinition: map[string]any{
			"app": "setters",
			"processes": map[string]any{
				"app":  "run-web",
				"back": "run-back",
				"foo":  "bar",
			},
		},
	}, cfg)
}

func TestSetStatics(t *testing.T) {
	cfg := NewConfig()
	cfg.SetStatics([]Static{{GuestPath: "/guest", UrlPrefix: "/prefix"}})
	assert.Equal(t, &Config{
		Statics: []Static{
			{GuestPath: "/guest", UrlPrefix: "/prefix"},
		},
		RawDefinition: map[string]any{
			"statics": []Static{
				{GuestPath: "/guest", UrlPrefix: "/prefix"},
			},
		},
	}, cfg)
}

func TestSetVolumes(t *testing.T) {
	cfg := NewConfig()
	cfg.SetVolumes([]Volume{{Source: "data", Destination: "/data"}})
	assert.Equal(t, &Config{
		Mounts: &Volume{Source: "data", Destination: "/data"},
		RawDefinition: map[string]any{
			"mounts": []Volume{
				{Source: "data", Destination: "/data"},
			},
		},
	}, cfg)
}

func TestSetKillSignal(t *testing.T) {
	cfg := NewConfig()
	cfg.SetKillSignal("TERM")
	assert.Equal(t, &Config{
		KillSignal: "TERM",
		RawDefinition: map[string]any{
			"kill_signal": "TERM",
		},
	}, cfg)
}
