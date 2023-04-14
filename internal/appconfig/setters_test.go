package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
)

func TestSettersWithService(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-service.toml")
	assert.NoError(t, err)

	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status")
	cfg.SetConcurrency(12, 34)

	assert.Equal(t, cfg.Services, []Service{{
		InternalPort: 1234,
		Protocol:     "tcp",
		Concurrency: &api.MachineServiceConcurrency{
			Type:      "connections",
			HardLimit: 34,
			SoftLimit: 12,
		},
		HTTPChecks: []*ServiceHTTPCheck{{
			Interval:          api.MustParseDuration("10s"),
			Timeout:           api.MustParseDuration("2s"),
			GracePeriod:       api.MustParseDuration("5s"),
			HTTPMethod:        api.Pointer("GET"),
			HTTPPath:          api.Pointer("/status"),
			HTTPProtocol:      api.Pointer("http"),
			HTTPTLSSkipVerify: api.Pointer(false),
		}},
	}})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
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
	})
}

func TestSettersWithHTTPService(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-httpservice.toml")
	require.NoError(t, err)

	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status")
	cfg.SetConcurrency(12, 34)

	assert.Empty(t, cfg.Services)
	assert.Equal(t, cfg.HTTPService, &HTTPService{
		InternalPort: 1234,
		Concurrency: &api.MachineServiceConcurrency{
			Type:      "connections",
			HardLimit: 34,
			SoftLimit: 12,
		},
	})
	assert.Equal(t, cfg.Checks, map[string]*ToplevelCheck{
		"status": {
			Port:              api.Pointer(1234),
			Type:              api.Pointer("http"),
			Interval:          api.MustParseDuration("10s"),
			Timeout:           api.MustParseDuration("2s"),
			GracePeriod:       api.MustParseDuration("5s"),
			HTTPMethod:        api.Pointer("GET"),
			HTTPPath:          api.Pointer("/status"),
			HTTPProtocol:      api.Pointer("http"),
			HTTPTLSSkipVerify: api.Pointer(false),
		},
	})
	// No need to test RawDefinition because http_service is machines only
}

func TestSettersWithouServices(t *testing.T) {
	cfg := NewConfig()
	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status")
	cfg.SetConcurrency(12, 34)
	assert.Nil(t, cfg.Services, nil)
	assert.Nil(t, cfg.HTTPService, nil)
	assert.Equal(t, cfg.RawDefinition, map[string]any{})
}

func TestSetEnvVariable(t *testing.T) {
	cfg := NewConfig()
	cfg.SetEnvVariable("a", "1")
	cfg.SetEnvVariables(map[string]string{"b": "2", "c": "3"})
	assert.Equal(t, cfg.Env, map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"env": map[string]string{
			"a": "1",
			"b": "2",
			"c": "3",
		},
	})
}

func TestReleaseCommand(t *testing.T) {
	cfg := NewConfig()
	cfg.SetReleaseCommand("/bin/app/run migrate")
	assert.Equal(t, cfg.Deploy, &Deploy{
		ReleaseCommand: "/bin/app/run migrate",
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"deploy": map[string]string{
			"release_command": "/bin/app/run migrate",
		},
	})
}

func TestReleaseCommand_DeploySectionExists(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-deploy.toml")
	assert.NoError(t, err)

	cfg.SetReleaseCommand("/bin/app/run migrate")
	assert.Equal(t, cfg.Deploy, &Deploy{
		ReleaseCommand: "/bin/app/run migrate",
		Strategy:       "immediate",
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"app": "setters",
		"deploy": map[string]any{
			"release_command": "/bin/app/run migrate",
			"strategy":        "immediate",
		},
	})
}

func TestSetDocker(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, cfg.Experimental, &Experimental{
		Entrypoint: []string{"/entry"},
		Cmd:        []string{"/cmd"},
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"experimental": map[string]string{
			"entrypoint": "/entry",
			"cmd":        "/cmd",
		},
	})
}

func TestSetDocker_ExperimentalSectionExists(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-experimental.toml")
	require.NoError(t, err)

	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, cfg.Experimental, &Experimental{
		Entrypoint: []string{"/entry"},
		Cmd:        []string{"/cmd"},
		Exec:       []string{"/exec"},
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"app": "setters",
		"experimental": map[string]any{
			"entrypoint": "/entry",
			"cmd":        "/cmd",
			"exec":       "/exec",
		},
	})
}

func TestSetProcesses(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, cfg.Experimental, &Experimental{
		Entrypoint: []string{"/entry"},
		Cmd:        []string{"/cmd"},
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"experimental": map[string]string{
			"entrypoint": "/entry",
			"cmd":        "/cmd",
		},
	})
}

func TestSetProcesses_SectionExists(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-processes.toml")
	require.NoError(t, err)

	cfg.SetProcess("app", "run-web")
	cfg.SetProcess("back", "run-back")
	assert.Equal(t, cfg.Processes, map[string]string{
		"app":  "run-web",
		"back": "run-back",
		"foo":  "bar",
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"app": "setters",
		"processes": map[string]any{
			"app":  "run-web",
			"back": "run-back",
			"foo":  "bar",
		},
	})
}

func TestSetStatics(t *testing.T) {
	cfg := NewConfig()
	cfg.SetStatics([]Static{{GuestPath: "/guest", UrlPrefix: "/prefix"}})
	assert.Equal(t, cfg.Statics, []Static{
		{GuestPath: "/guest", UrlPrefix: "/prefix"},
	})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"statics": []Static{
			{GuestPath: "/guest", UrlPrefix: "/prefix"},
		},
	})
}

func TestSetVolumes(t *testing.T) {
	cfg := NewConfig()
	cfg.SetVolumes([]Mount{{Source: "data", Destination: "/data"}})
	assert.Equal(t, cfg.Mounts, &Mount{Source: "data", Destination: "/data"})
	assert.Equal(t, cfg.RawDefinition, map[string]any{
		"mounts": []Mount{
			{Source: "data", Destination: "/data"},
		},
	})
}

func TestSetKillSignal(t *testing.T) {
	cfg := NewConfig()
	cfg.SetKillSignal("TERM")
	assert.Equal(t, cfg.KillSignal, api.Pointer("TERM"))
	assert.Equal(t, cfg.RawDefinition, map[string]any{"kill_signal": "TERM"})
}
