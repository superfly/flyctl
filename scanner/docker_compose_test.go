package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureDockerCompose(t *testing.T) {
	t.Run("detects docker-compose.yml", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a simple docker-compose.yml
		composeContent := `version: '3.8'
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
    environment:
      - ENV_VAR=value
    depends_on:
      - db
  db:
    image: postgres:13
    environment:
      - POSTGRES_PASSWORD=secret
  redis:
    image: redis:alpine
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify basic properties
		assert.Equal(t, "Docker Compose", srcInfo.Family)
		assert.Equal(t, "3.8", srcInfo.Version)
		assert.Equal(t, 80, srcInfo.Port)

		// Verify containers were created (excluding database services)
		assert.Len(t, srcInfo.Containers, 1)
		assert.Equal(t, "web", srcInfo.Containers[0].Name)
		assert.Equal(t, "nginx:latest", srcInfo.Containers[0].Image)

		// Verify entrypoint script is set for service discovery
		assert.Equal(t, []string{"/fly-entrypoint.sh"}, srcInfo.Containers[0].Entrypoint)

		// Verify entrypoint script file is included
		assert.Len(t, srcInfo.Files, 1)
		assert.Equal(t, "/fly-entrypoint.sh", srcInfo.Files[0].Path)
		assert.Contains(t, string(srcInfo.Files[0].Contents), "127.0.0.1    web")

		// Verify database detection
		assert.Equal(t, DatabaseKindPostgres, srcInfo.DatabaseDesired)
		assert.True(t, srcInfo.RedisDesired)

		// Verify dependencies - database dependency should be filtered out
		assert.Len(t, srcInfo.Containers[0].DependsOn, 0)
	})

	t.Run("handles build context", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with build context
		composeContent := `version: '3'
services:
  app:
    build: .
    ports:
      - "3000:3000"
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify container with build context
		assert.Len(t, srcInfo.Containers, 1)
		assert.Equal(t, "app", srcInfo.Containers[0].Name)
		assert.Equal(t, tmpDir, srcInfo.Containers[0].BuildContext)
		assert.Equal(t, 3000, srcInfo.Port)

		// Verify entrypoint script is set
		assert.Equal(t, []string{"/fly-entrypoint.sh"}, srcInfo.Containers[0].Entrypoint)
	})

	t.Run("handles health checks", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with health check
		composeContent := `version: '3'
services:
  app:
    image: myapp:latest
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify health check
		assert.Len(t, srcInfo.Containers, 1)
		require.NotNil(t, srcInfo.Containers[0].HealthCheck)
		assert.Equal(t, []string{"CMD", "curl", "-f", "http://localhost/health"}, srcInfo.Containers[0].HealthCheck.Test)
		assert.Equal(t, "30s", srcInfo.Containers[0].HealthCheck.Interval)
		assert.Equal(t, "10s", srcInfo.Containers[0].HealthCheck.Timeout)
		assert.Equal(t, 3, srcInfo.Containers[0].HealthCheck.Retries)
		assert.Equal(t, "40s", srcInfo.Containers[0].HealthCheck.StartPeriod)
	})

	t.Run("returns nil for non-compose projects", func(t *testing.T) {
		// Create temporary directory without docker-compose.yml
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		assert.NoError(t, err)
		assert.Nil(t, srcInfo)
	})

	t.Run("preserves non-database dependencies", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with mixed dependencies
		composeContent := `version: '3'
services:
  web:
    image: nginx:latest
    depends_on:
      - api
      - db
      - cache
  api:
    image: myapi:latest
  db:
    image: postgres:13
  cache:
    image: redis:alpine
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Should have 2 containers (web and api, excluding db and cache)
		assert.Len(t, srcInfo.Containers, 2)

		// Find the web container
		var webContainer *Container
		for i := range srcInfo.Containers {
			if srcInfo.Containers[i].Name == "web" {
				webContainer = &srcInfo.Containers[i]
				break
			}
		}
		require.NotNil(t, webContainer)

		// Web should only depend on api (db and cache are filtered out)
		assert.Len(t, webContainer.DependsOn, 1)
		assert.Equal(t, "api", webContainer.DependsOn[0].Name)
		assert.Equal(t, "started", webContainer.DependsOn[0].Condition)

		// Verify database services were detected
		assert.Equal(t, DatabaseKindPostgres, srcInfo.DatabaseDesired)
		assert.True(t, srcInfo.RedisDesired)
	})
}
