package flyctl

import (
	"bytes"
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

func TestGetAndSetEnvVariables(t *testing.T) {
	cfg := NewAppConfig()

	cfg.SetEnvVariable("A", "B")
	cfg.SetEnvVariable("C", "D")

	assert.Equal(t, map[string]string{"A": "B", "C": "D"}, cfg.GetEnvVariables())

	buf := &bytes.Buffer{}

	if err := cfg.WriteTo(buf, TOMLFormat); err != nil {
		assert.NoError(t, err)
	}

	cfg2 := NewAppConfig()

	if err := cfg2.unmarshalTOML(bytes.NewReader(buf.Bytes())); err != nil {
		assert.NoError(t, err)
	}

	assert.Equal(t, cfg.GetEnvVariables(), cfg2.GetEnvVariables())
}
