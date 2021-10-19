package flyctl

import (
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
