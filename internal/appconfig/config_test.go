package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
)

func TestGetAndSetEnvVariables(t *testing.T) {
	cfg := NewConfig()
	cfg.SetEnvVariable("A", "B")
	cfg.SetEnvVariable("C", "D")
	assert.Equal(t, map[string]string{"A": "B", "C": "D"}, cfg.Env)

	bytes, err := cfg.marshalTOML()
	assert.NoError(t, err)
	cfg2, err := unmarshalTOML(bytes)
	assert.NoError(t, err)
	assert.Equal(t, cfg.Env, cfg2.Env)
}

func TestConfigDockerGetters(t *testing.T) {
	validCfg := Config{
		Build: &Build{
			Dockerfile:        "some_dockerfile",
			Ignorefile:        "some_ignore_file",
			DockerBuildTarget: "some_build_target",
		},
	}

	assert.Equal(t, validCfg.Dockerfile(), "some_dockerfile")
	assert.Equal(t, validCfg.Ignorefile(), "some_ignore_file")
	assert.Equal(t, validCfg.DockerBuildTarget(), "some_build_target")

	var nilCfg *Config

	assert.Equal(t, nilCfg.Dockerfile(), "")
	assert.Equal(t, nilCfg.Ignorefile(), "")
	assert.Equal(t, nilCfg.DockerBuildTarget(), "")
}

func TestNilBuildStrategy(t *testing.T) {
	var nilCfg *Config
	assert.Equal(t, 0, len(nilCfg.BuildStrategies()))
}

func TestDefaultBuildStrategy(t *testing.T) {
	cfg := Config{
		Build: &Build{},
	}

	assert.Equal(t, 0, len(cfg.BuildStrategies()))
}

func TestOneBuildStrategy(t *testing.T) {
	cfg := Config{
		Build: &Build{
			Builder: "heroku/buildpacks:20",
		},
	}

	assert.Equal(t, 1, len(cfg.BuildStrategies()))
}

func TestManyBuildStrategies(t *testing.T) {
	cfg := Config{
		Build: &Build{
			Dockerfile: "my-df",
			Builder:    "heroku/buildpacks:20",
			Builtin:    "node",
			Image:      "nginx",
		},
	}

	assert.Equal(t, 4, len(cfg.BuildStrategies()))
}

func TestConfigPortGetter(t *testing.T) {
	type testcase struct {
		name         string
		config       Config
		expectedPort int
	}

	testcases := []testcase{
		{
			name:         "no port set in services",
			expectedPort: 0,
			config:       Config{},
		},
		{
			name:         "port set in services",
			expectedPort: 1000,
			config: Config{
				Services: []Service{{InternalPort: 1000}},
			},
		},
		{
			name:         "port set in services and http services",
			expectedPort: 3000,
			config: Config{
				HTTPService: &HTTPService{
					InternalPort: 3000,
				},
				Services: []Service{
					{
						InternalPort: 1000,
					},
				},
			},
		},
		{
			name:         "port set in http services",
			expectedPort: 9876,
			config: Config{
				HTTPService: &HTTPService{
					InternalPort: 9876,
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedPort, tc.config.InternalPort())
		})
	}
}

// This can't go in helpers/clone_test.go because of an import cycle
func TestCloneAppconfig(t *testing.T) {
	config := &Config{
		AppName: "testcfg",
		Mounts: []Mount{{
			Source:      "src",
			Destination: "dst",
		}},
		HTTPService: &HTTPService{
			InternalPort: 100,
		},
		defaultGroupName: "some-group",
	}

	cloned := helpers.Clone(config)

	assert.Equal(t, config, cloned)

	config.HTTPService.InternalPort = 50

	assert.Equal(t, 100, cloned.HTTPService.InternalPort,
		"expected deep copy, but cloned object was modified by change to original config")
}

func TestDetermineIPType(t *testing.T) {
	port80 := 80
	port443 := 443

	cfg1 := NewConfig()
	cfg1.Services = []Service{{Protocol: "tcp", Ports: []fly.MachinePort{
		{Port: &port80, Handlers: []string{"http"}},
	}}}
	assert.Equal(t, "shared", cfg1.DetermineIPType("public"))
	assert.Equal(t, "private", cfg1.DetermineIPType("private"))

	cfg2 := NewConfig()
	cfg2.Services = []Service{{Protocol: "tcp", Ports: []fly.MachinePort{
		{Port: &port443, Handlers: []string{"tls", "http"}},
	}}}
	assert.Equal(t, "shared", cfg2.DetermineIPType("public"))

	cfg3 := NewConfig()
	cfg3.Services = []Service{{Protocol: "tcp", Ports: []fly.MachinePort{
		{Port: &port443, Handlers: []string{"http", "tls"}},
	}}}
	assert.Equal(t, "shared", cfg3.DetermineIPType("public"))

	cfg4 := NewConfig()
	cfg4.Services = []Service{{Protocol: "tcp", Ports: []fly.MachinePort{
		{Port: &port443, Handlers: []string{"tls", "weird"}},
	}}}
	assert.Equal(t, "dedicated", cfg4.DetermineIPType("public"))
	assert.Equal(t, "private", cfg4.DetermineIPType("private"))

	cfg5 := NewConfig()
	cfg5.Services = []Service{{Protocol: "tcp", Ports: []fly.MachinePort{
		{Port: &port443, Handlers: []string{"tls"}},
	}}}
	assert.Equal(t, "dedicated", cfg5.DetermineIPType("public"))

	cfg6 := NewConfig()
	cfg6.Services = []Service{{Protocol: "udp", Ports: []fly.MachinePort{
		{Port: &port443, Handlers: []string{"tls", "http"}},
	}}}
	assert.Equal(t, "dedicated", cfg6.DetermineIPType("public"))
}

func TestURL(t *testing.T) {
	cfg := NewConfig()
	cfg.AppName = "test"
	cfg.HTTPService = &HTTPService{InternalPort: 8080}
	assert.Equal(t, "https://test.fly.dev/", cfg.URL().String())

	// Prefer https on 443 over http on 80
	cfg = NewConfig()
	cfg.AppName = "test"
	cfg.Services = []Service{{
		Protocol: "tcp",
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(80), Handlers: []string{"http"},
		}, {
			Port: fly.Pointer(443), Handlers: []string{"http", "tls"},
		}},
	}}
	assert.Equal(t, "https://test.fly.dev/", cfg.URL().String())

	// port 443 is not http, only port 80 is.
	cfg = NewConfig()
	cfg.AppName = "test"
	cfg.Services = []Service{{
		Protocol: "tcp",
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(80), Handlers: []string{"http"},
		}, {
			Port: fly.Pointer(443), Handlers: []string{"tls"},
		}},
	}}
	assert.Equal(t, "http://test.fly.dev/", cfg.URL().String())

	// prefer standard http port over non standard https port
	cfg = NewConfig()
	cfg.AppName = "test"
	cfg.Services = []Service{{
		Protocol: "tcp",
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(80), Handlers: []string{"http"},
		}, {
			Port: fly.Pointer(3443), Handlers: []string{"tls", "http"},
		}},
	}}
	assert.Equal(t, "http://test.fly.dev/", cfg.URL().String())

	// prefer non standard https port over non standard http port
	cfg = NewConfig()
	cfg.AppName = "test"
	cfg.Services = []Service{{
		Protocol: "tcp",
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(8080), Handlers: []string{"http"},
		}, {
			Port: fly.Pointer(3443), Handlers: []string{"tls", "http"},
		}},
	}}
	assert.Equal(t, "https://test.fly.dev:3443/", cfg.URL().String())

	// Use non standard http port as last meassure
	cfg = NewConfig()
	cfg.AppName = "test"
	cfg.Services = []Service{{
		Protocol: "tcp",
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(8080), Handlers: []string{"http"},
		}},
	}}
	assert.Equal(t, "http://test.fly.dev:8080/", cfg.URL().String())

	// Otherwise return an empty string so caller knows there is no http service
	cfg = NewConfig()
	cfg.AppName = "test"
	cfg.Services = []Service{{
		Protocol: "tcp",
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(80), Handlers: []string{"fancy"},
		}, {
			Port: fly.Pointer(443), Handlers: []string{"foo"},
		}},
	}}
	assert.Nil(t, cfg.URL())
}
