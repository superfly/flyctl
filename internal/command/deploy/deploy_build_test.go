package deploy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
}

func TestResolveDockerfilePath(t *testing.T) {
	t.Run("local path", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "fly.toml")

		cfg := &appconfig.Config{}
		cfg.SetConfigFilePath(configPath)
		cfg.Build = &appconfig.Build{
			Dockerfile: "Dockerfile.custom",
		}

		ctx := context.Background()
		path, err := resolveDockerfilePath(ctx, cfg)

		require.NoError(t, err)
		expectedPath := filepath.Join(dir, "Dockerfile.custom")
		assert.Equal(t, expectedPath, path)
	})

	t.Run("URL path", func(t *testing.T) {
		// Create a test server that serves a Dockerfile
		dockerfileContent := `FROM alpine:latest
RUN apk add --no-cache curl
ENTRYPOINT ["sh"]`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, dockerfileContent)
		}))
		defer server.Close()

		cfg := &appconfig.Config{}
		cfg.Build = &appconfig.Build{
			Dockerfile: server.URL + "/Dockerfile",
		}

		ctx := context.Background()
		path, err := resolveDockerfilePath(ctx, cfg)

		require.NoError(t, err)
		assert.NotEmpty(t, path)

		// Verify the file exists and contains the expected content
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, dockerfileContent, string(content))

		// Clean up the temporary file
		os.Remove(path)
	})

	t.Run("URL path with 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		cfg := &appconfig.Config{}
		cfg.Build = &appconfig.Build{
			Dockerfile: server.URL + "/nonexistent",
		}

		ctx := context.Background()
		_, err := resolveDockerfilePath(ctx, cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 404")
	})
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"https://example.com/Dockerfile", true},
		{"http://example.com/Dockerfile", true},
		{"ftp://example.com/Dockerfile", false},
		{"./Dockerfile", false},
		{"/path/to/Dockerfile", false},
		{"Dockerfile", false},
		{"not-a-url", false},
		{"https://", false}, // invalid URL
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			result := isURL(test.path)
			assert.Equal(t, test.expected, result)
		})
	}
}
