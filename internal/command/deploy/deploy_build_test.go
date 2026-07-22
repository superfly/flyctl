package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/containerconfig"
	"github.com/superfly/flyctl/internal/state"
)

func TestMultipleDockerfile(t *testing.T) {
	dir := t.TempDir()

	dockerfile, err := os.Create(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	defer dockerfile.Close() // skipcq: GO-S2307

	flyToml, err := os.Create(filepath.Join(dir, "fly.production.toml"))
	require.NoError(t, err)
	defer flyToml.Close() // skipcq: GO-S2307

	cfg, err := appconfig.LoadConfig(flyToml.Name())
	require.NoError(t, err)
	cfg.Build = &appconfig.Build{
		Dockerfile: "Dockerfile.from-fly-toml",
	}

	ctx := state.WithWorkingDirectory(context.Background(), dir)
	err = multipleDockerfile(ctx, &appconfig.Config{})

	assert.NoError(t, err)

	err = multipleDockerfile(ctx, cfg)
	assert.ErrorContains(t, err, "fly.production.toml")
}

// writeComposeApp writes a fly.toml with [build.compose] and a compose.yml into
// a temp dir, plus an optional stray Dockerfile, and returns the loaded config.
func writeComposeApp(t *testing.T, composeYML string, withDockerfile bool) *appconfig.Config {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(composeYML), 0644))
	if withDockerfile {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0644))
	}

	flyToml := filepath.Join(dir, "fly.toml")
	require.NoError(t, os.WriteFile(flyToml, []byte(`app = "compose-test"

[build.compose]
  file = "compose.yml"
`), 0644))

	cfg, err := appconfig.LoadConfig(flyToml)
	require.NoError(t, err)

	return cfg
}

func TestComposeBuildInfo_NoBuildService(t *testing.T) {
	// Every service uses a pre-built image and a stray Dockerfile is present:
	// the gate must report "uses compose, no build needed".
	cfg := writeComposeApp(t, `services:
  web:
    image: nginx:latest
  db:
    image: postgres:14
`, true /* stray Dockerfile present */)

	usesCompose, cb, err := composeBuildInfo(cfg)
	require.NoError(t, err)
	assert.True(t, usesCompose, "expected compose to be detected")
	assert.Nil(t, cb, "expected no build directive when all services use images")
}

func TestComposeBuildInfo_WithBuildService(t *testing.T) {
	cfg := writeComposeApp(t, `services:
  app:
    build:
      context: ./src
      dockerfile: Dockerfile.custom
  cache:
    image: redis:alpine
`, false)

	usesCompose, cb, err := composeBuildInfo(cfg)
	require.NoError(t, err)
	assert.True(t, usesCompose)
	require.NotNil(t, cb)
	assert.Equal(t, "./src", cb.Context)
	assert.Equal(t, "Dockerfile.custom", cb.Dockerfile)
}

func TestComposeBuildInfo_NotCompose(t *testing.T) {
	// A plain Dockerfile app must be reported as not using compose.
	dir := t.TempDir()
	flyToml := filepath.Join(dir, "fly.toml")
	require.NoError(t, os.WriteFile(flyToml, []byte(`app = "plain"`+"\n"), 0644))
	cfg, err := appconfig.LoadConfig(flyToml)
	require.NoError(t, err)

	usesCompose, cb, err := composeBuildInfo(cfg)
	require.NoError(t, err)
	assert.False(t, usesCompose)
	assert.Nil(t, cb)
}

func TestApplyComposeBuild_ContextAndDockerfile(t *testing.T) {
	cfg := writeComposeApp(t, `services:
  app:
    build:
      context: ./src
      dockerfile: Dockerfile.custom
`, false)
	base := filepath.Dir(cfg.ConfigFilePath())

	opts := &imgsrc.ImageOptions{WorkingDir: base}
	cb := &containerconfig.ComposeBuild{Context: "./src", Dockerfile: "Dockerfile.custom"}

	require.NoError(t, applyComposeBuild(opts, cfg, cb))

	wantWorkDir, _ := filepath.Abs(filepath.Join(base, "src"))
	wantDockerfile, _ := filepath.Abs(filepath.Join(base, "src", "Dockerfile.custom"))
	assert.Equal(t, wantWorkDir, opts.WorkingDir)
	assert.Equal(t, wantDockerfile, opts.DockerfilePath)
}

func TestApplyComposeBuild_DockerfileOnly(t *testing.T) {
	cfg := writeComposeApp(t, `services:
  app:
    build:
      dockerfile: Dockerfile.custom
`, false)
	base := filepath.Dir(cfg.ConfigFilePath())

	opts := &imgsrc.ImageOptions{WorkingDir: base}
	cb := &containerconfig.ComposeBuild{Dockerfile: "Dockerfile.custom"}

	require.NoError(t, applyComposeBuild(opts, cfg, cb))

	wantDockerfile, _ := filepath.Abs(filepath.Join(base, "Dockerfile.custom"))
	assert.Equal(t, base, opts.WorkingDir, "working dir unchanged when no context")
	assert.Equal(t, wantDockerfile, opts.DockerfilePath)
}
