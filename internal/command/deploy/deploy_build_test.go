package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
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

	t.Run("redacts credentials in URL warning", func(t *testing.T) {
		cfg.Build.Dockerfile = "https://" + "user:password@" + "example.com/Dockerfile?token=secret#fragment"

		err := multipleDockerfile(ctx, cfg)

		require.Error(t, err)
		assert.ErrorContains(t, err, "https://example.com/Dockerfile")
		assert.NotContains(t, err.Error(), "user")
		assert.NotContains(t, err.Error(), "password")
		assert.NotContains(t, err.Error(), "token")
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "fragment")
	})
}

func TestResolveDockerfilePath(t *testing.T) {
	t.Run("relative config path", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &appconfig.Config{
			Build: &appconfig.Build{Dockerfile: "Dockerfile.custom"},
		}
		cfg.SetConfigFilePath(filepath.Join(dir, "fly.toml"))

		got, err := resolveDockerfilePath(context.Background(), cfg)

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "Dockerfile.custom"), got)
	})

	t.Run("URL remains unchanged", func(t *testing.T) {
		const dockerfileURL = "https://example.com/Dockerfile?token=secret"
		cfg := &appconfig.Config{
			Build: &appconfig.Build{Dockerfile: dockerfileURL},
		}

		got, err := resolveDockerfilePath(context.Background(), cfg)

		require.NoError(t, err)
		assert.Equal(t, dockerfileURL, got)
	})

}
