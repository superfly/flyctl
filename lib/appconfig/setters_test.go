package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
)

func TestSettersWithService(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-service.toml")
	assert.NoError(t, err)

	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status", nil)
	cfg.SetConcurrency(12, 34)

	assert.Equal(t, cfg.Services, []Service{{
		InternalPort: 1234,
		Protocol:     "tcp",
		Concurrency: &fly.MachineServiceConcurrency{
			Type:      "connections",
			HardLimit: 34,
			SoftLimit: 12,
		},
		HTTPChecks: []*ServiceHTTPCheck{{
			Interval:          fly.MustParseDuration("10s"),
			Timeout:           fly.MustParseDuration("2s"),
			GracePeriod:       fly.MustParseDuration("5s"),
			HTTPMethod:        fly.Pointer("GET"),
			HTTPPath:          fly.Pointer("/status"),
			HTTPProtocol:      fly.Pointer("http"),
			HTTPTLSSkipVerify: fly.Pointer(false),
		}},
	}})
}

func TestSettersWithHTTPService(t *testing.T) {
	cfg, err := LoadConfig("./testdata/setters-httpservice.toml")
	require.NoError(t, err)

	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status", nil)
	cfg.SetConcurrency(12, 34)

	assert.Empty(t, cfg.Services)
	assert.Equal(t, cfg.HTTPService, &HTTPService{
		InternalPort: 1234,
		Concurrency: &fly.MachineServiceConcurrency{
			Type:      "connections",
			HardLimit: 34,
			SoftLimit: 12,
		},
		HTTPChecks: []*ServiceHTTPCheck{{
			Interval:          fly.MustParseDuration("10s"),
			Timeout:           fly.MustParseDuration("2s"),
			GracePeriod:       fly.MustParseDuration("5s"),
			HTTPMethod:        fly.Pointer("GET"),
			HTTPPath:          fly.Pointer("/status"),
			HTTPProtocol:      fly.Pointer("http"),
			HTTPTLSSkipVerify: fly.Pointer(false),
		}},
	})
}

func TestSettersWithouServices(t *testing.T) {
	cfg := NewConfig()
	cfg.SetInternalPort(1234)
	cfg.SetHttpCheck("/status", nil)
	cfg.SetConcurrency(12, 34)
	assert.Nil(t, cfg.Services, nil)
	assert.Nil(t, cfg.HTTPService, nil)
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
}

func TestReleaseCommand(t *testing.T) {
	cfg := NewConfig()
	cfg.SetReleaseCommand("/bin/app/run migrate")
	assert.Equal(t, cfg.Deploy, &Deploy{
		ReleaseCommand: "/bin/app/run migrate",
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
}

func TestSetDocker(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, cfg.Experimental, &Experimental{
		Entrypoint: []string{"/entry"},
		Cmd:        []string{"/cmd"},
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
}

func TestSetProcesses(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDockerEntrypoint("/entry")
	cfg.SetDockerCommand("/cmd")
	assert.Equal(t, cfg.Experimental, &Experimental{
		Entrypoint: []string{"/entry"},
		Cmd:        []string{"/cmd"},
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
}

func TestSetStatics(t *testing.T) {
	cfg := NewConfig()
	cfg.SetStatics([]Static{{GuestPath: "/guest", UrlPrefix: "/prefix"}})
	assert.Equal(t, cfg.Statics, []Static{
		{GuestPath: "/guest", UrlPrefix: "/prefix"},
	})
}

func TestSetVolumes(t *testing.T) {
	cfg := NewConfig()
	cfg.SetMounts([]Mount{{Source: "data", Destination: "/data"}})
	assert.Equal(t, cfg.Mounts, []Mount{{Source: "data", Destination: "/data"}})
}

func TestSetKillSignal(t *testing.T) {
	cfg := NewConfig()
	cfg.SetKillSignal("TERM")
	assert.Equal(t, cfg.KillSignal, fly.Pointer("TERM"))
}
