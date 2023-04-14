package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		RawDefinition: map[string]any{
			"mounts": []Volume{
				{
					Source:      "src-raw",
					Destination: "dst-raw",
				},
				{
					Source:      "src2",
					Destination: "dst2",
				},
			},
		},
		Mounts: &Volume{
			Source:      "src",
			Destination: "dst",
		},
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
