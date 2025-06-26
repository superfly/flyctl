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

	t.Run("handles single build section correctly", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with one build service and one image service
		composeContent := `version: '3'
services:
  web:
    build: .
    ports:
      - "8080:8080"
  worker:
    image: nginx:latest
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Should have 2 containers
		assert.Len(t, srcInfo.Containers, 2)

		// Container field should be set to the build service
		assert.Equal(t, "web", srcInfo.Container)

		// Verify containers
		var webContainer, workerContainer *Container
		for i := range srcInfo.Containers {
			if srcInfo.Containers[i].Name == "web" {
				webContainer = &srcInfo.Containers[i]
			} else if srcInfo.Containers[i].Name == "worker" {
				workerContainer = &srcInfo.Containers[i]
			}
		}
		require.NotNil(t, webContainer)
		require.NotNil(t, workerContainer)

		// Web container should have build context
		assert.Equal(t, tmpDir, webContainer.BuildContext)
		assert.Equal(t, "", webContainer.Image)

		// Worker container should have image
		assert.Equal(t, "nginx:latest", workerContainer.Image)
		assert.Equal(t, "", workerContainer.BuildContext)
	})

	t.Run("errors on multiple build sections", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with multiple build services
		composeContent := `version: '3'
services:
  web:
    build: .
    ports:
      - "8080:8080"
  api:
    build: ./api
  worker:
    image: nginx:latest
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner - should error
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		assert.Error(t, err)
		assert.Nil(t, srcInfo)
		assert.Contains(t, err.Error(), "multiple services with build sections found")
	})

	t.Run("extracts database credentials as secrets", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with database credentials
		composeContent := `version: '3'
services:
  web:
    image: myapp:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgresql://user:pass@db:5432/myapp
      - REDIS_URL=redis://cache:6379
      - API_KEY=not-a-database-key
      - DB_PASSWORD=secretpass
      - NORMAL_ENV=value
  db:
    image: postgres:13
    environment:
      - POSTGRES_PASSWORD=secret
  cache:
    image: redis:alpine
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify secrets were extracted
		// DATABASE_URL and REDIS_URL should NOT be in secrets because Fly.io managed databases are detected
		assert.Len(t, srcInfo.Secrets, 1) // Only DB_PASSWORD

		// Check specific secrets
		secretKeys := make(map[string]string)
		for _, secret := range srcInfo.Secrets {
			secretKeys[secret.Key] = secret.Value
		}

		// DATABASE_URL and REDIS_URL should NOT be in secrets (Fly will provide them)
		assert.NotContains(t, secretKeys, "DATABASE_URL")
		assert.NotContains(t, secretKeys, "REDIS_URL")

		// DB_PASSWORD should still be in secrets
		assert.Contains(t, secretKeys, "DB_PASSWORD")
		assert.Equal(t, "secretpass", secretKeys["DB_PASSWORD"])

		// Verify non-database env vars remain in container environment
		assert.Len(t, srcInfo.Containers, 1)
		webContainer := srcInfo.Containers[0]
		assert.Equal(t, "not-a-database-key", webContainer.Env["API_KEY"])
		assert.Equal(t, "value", webContainer.Env["NORMAL_ENV"])

		// Verify database env vars were removed from container environment
		assert.NotContains(t, webContainer.Env, "DATABASE_URL")
		assert.NotContains(t, webContainer.Env, "REDIS_URL")
		assert.NotContains(t, webContainer.Env, "DB_PASSWORD")

		// Verify container has the secrets it needs access to
		assert.Len(t, webContainer.Secrets, 1)
		assert.Contains(t, webContainer.Secrets, "DB_PASSWORD")
		// DATABASE_URL and REDIS_URL are not in secrets list because Fly provides them

		// Verify database services were detected
		assert.Equal(t, DatabaseKindPostgres, srcInfo.DatabaseDesired)
		assert.True(t, srcInfo.RedisDesired)
	})

	t.Run("extracts all database credentials when no managed databases detected", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml without database services (no managed db will be created)
		composeContent := `version: '3'
services:
  web:
    image: myapp:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgresql://user:pass@external-db:5432/myapp
      - REDIS_URL=redis://external-cache:6379
      - DB_PASSWORD=secretpass
      - NORMAL_ENV=value
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify NO database services were detected
		assert.Equal(t, DatabaseKindNone, srcInfo.DatabaseDesired)
		assert.False(t, srcInfo.RedisDesired)

		// Verify ALL database secrets were extracted (including DATABASE_URL and REDIS_URL)
		assert.Len(t, srcInfo.Secrets, 3) // DATABASE_URL, REDIS_URL, DB_PASSWORD

		// Check specific secrets
		secretKeys := make(map[string]string)
		for _, secret := range srcInfo.Secrets {
			secretKeys[secret.Key] = secret.Value
		}

		// When no managed databases are detected, these should be in secrets
		assert.Contains(t, secretKeys, "DATABASE_URL")
		assert.Equal(t, "postgresql://user:pass@external-db:5432/myapp", secretKeys["DATABASE_URL"])
		assert.Contains(t, secretKeys, "REDIS_URL")
		assert.Equal(t, "redis://external-cache:6379", secretKeys["REDIS_URL"])
		assert.Contains(t, secretKeys, "DB_PASSWORD")
		assert.Equal(t, "secretpass", secretKeys["DB_PASSWORD"])

		// Verify database env vars were removed from container environment
		assert.Len(t, srcInfo.Containers, 1)
		webContainer := srcInfo.Containers[0]
		assert.NotContains(t, webContainer.Env, "DATABASE_URL")
		assert.NotContains(t, webContainer.Env, "REDIS_URL")
		assert.NotContains(t, webContainer.Env, "DB_PASSWORD")
		assert.Equal(t, "value", webContainer.Env["NORMAL_ENV"])

		// Verify container has all the secrets it needs access to
		assert.Len(t, webContainer.Secrets, 3)
		assert.Contains(t, webContainer.Secrets, "DATABASE_URL")
		assert.Contains(t, webContainer.Secrets, "REDIS_URL")
		assert.Contains(t, webContainer.Secrets, "DB_PASSWORD")
	})
}
