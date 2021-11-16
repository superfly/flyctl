package flyctl

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
)

func TestLoadTOMLAppConfigWithAppName(t *testing.T) {
	path := "./testdata/app-name.toml"
	p, err := LoadAppConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.AppName, "test-app")
}

func TestLoadTOMLAppConfigWithBuilderName(t *testing.T) {
	path := "./testdata/build.toml"
	p, err := LoadAppConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Builder, "builder/name")
}

func TestLoadTOMLAppConfigWithImage(t *testing.T) {
	path := "./testdata/image.toml"
	p, err := LoadAppConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Image, "image/name")
}

func TestLoadTOMLAppConfigWithDockerfile(t *testing.T) {
	path := "./testdata/docker.toml"
	p, err := LoadAppConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Dockerfile, "./Dockerfile")
}

func TestLoadTOMLAppConfigWithBuilderNameAndArgs(t *testing.T) {
	path := "./testdata/build-with-args.toml"
	p, err := LoadAppConfig(path)
	assert.NoError(t, err)
	assert.Equal(t, p.Build.Args, map[string]string{"A": "B", "C": "D"})
}

func TestLoadTOMLAppConfigWithServices(t *testing.T) {
	path := "./testdata/services.toml"
	p, err := LoadAppConfig(path)

	rawData := map[string]interface{}{}
	toml.DecodeFile("./testdata/services.toml", &rawData)
	delete(rawData, "app")
	delete(rawData, "build")

	assert.NoError(t, err)
	assert.Equal(t, p.Definition, rawData)
}

func TestLoadTOMLAppConfigWithEnvVars(t *testing.T) {
	const path = "./testdata/env-vars.toml"
	p, err := LoadAppConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "debug", p.Env["LOG_LEVEL"])
	assert.Equal(t, "production", p.Env["RAILS_ENV"])

	p.SetEnvVariable("LOG_LEVEL", "info")

	assert.Equal(t, "info", p.Env["LOG_LEVEL"])
	assert.Equal(t, "production", p.Env["RAILS_ENV"])

	p.SetEnvVariables(map[string]string{"RAILS_ENV": "development"})

	assert.Equal(t, "info", p.Env["LOG_LEVEL"])
	assert.Equal(t, "development", p.Env["RAILS_ENV"])
}

func TestEnvironment(t *testing.T) {
	env := Environment{}
	env.Set("k1", "v1")
	assert.Equal(t, "v1", env["k1"])
	assert.Equal(t, "v1", env.Get("k1"))
	env.Set("k2", "v2")
	env.MultiSet(map[string]string{"k2": "v2.2", "k3": "v3"})
	assert.Equal(t, "v1", env.Get("k1"))
	assert.Equal(t, "v2.2", env.Get("k2"))
	assert.Equal(t, "v3", env.Get("k3"))
}
