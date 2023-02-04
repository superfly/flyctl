package app

import (
	"testing"

	"github.com/BurntSushi/toml"
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

func TestLoadTOMLAppConfigWithServices(t *testing.T) {
	const path = "./testdata/services.toml"
	p, err := LoadConfig(path)
	assert.NoError(t, err)

	rawData := &api.Definition{}
	toml.DecodeFile("./testdata/services.toml", rawData)
	delete(*rawData, "app")
	delete(*rawData, "build")

	definition, err := p.ToDefinition()
	assert.NoError(t, err)
	assert.Equal(t, definition, rawData)
}

func TestLoadTOMLAppConfigOldFormat(t *testing.T) {
	const path = "./testdata/old-format.toml"
	cfg, err := LoadConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, cfg, &Config{
		FlyTomlPath: "./testdata/old-format.toml",
		AppName:     "foo",
		Env: map[string]string{
			"FOO": "STRING",
			"BAR": "123",
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
						Interval: api.MustParseDuration("30s"),
						Timeout:  api.MustParseDuration("4s"),
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
	})
}
