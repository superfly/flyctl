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

		// Verify all containers are returned during scanning (filtering happens in callback)
		assert.Len(t, srcInfo.Containers, 3)

		// Find the web container
		var webContainer *Container
		for i := range srcInfo.Containers {
			if srcInfo.Containers[i].Name == "web" {
				webContainer = &srcInfo.Containers[i]
				break
			}
		}
		require.NotNil(t, webContainer, "web container should be found")

		assert.Equal(t, "web", webContainer.Name)
		assert.Equal(t, "nginx:latest", webContainer.Image)

		// Verify entrypoint is NOT set for nginx (no explicit command/entrypoint)
		assert.Nil(t, webContainer.Entrypoint)
		assert.True(t, webContainer.UseImageDefaults)

		// Verify no entrypoint script file is created (not needed for image-only containers)
		assert.Len(t, srcInfo.Files, 0)

		// Verify database detection
		assert.Equal(t, DatabaseKindPostgres, srcInfo.DatabaseDesired)
		assert.True(t, srcInfo.RedisDesired)

		// Verify dependencies are preserved during scanning (filtering happens in callback)
		assert.Len(t, webContainer.DependsOn, 1)
		assert.Equal(t, "db", webContainer.DependsOn[0].Name)
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

		// Verify entrypoint is NOT set for build context without explicit command
		assert.Nil(t, srcInfo.Containers[0].Entrypoint)
		assert.True(t, srcInfo.Containers[0].UseImageDefaults)
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

		// Should have 4 containers (all containers returned during scanning)
		assert.Len(t, srcInfo.Containers, 4)

		// Find the web container
		var webContainer *Container
		for i := range srcInfo.Containers {
			if srcInfo.Containers[i].Name == "web" {
				webContainer = &srcInfo.Containers[i]
				break
			}
		}
		require.NotNil(t, webContainer)

		// Web should depend on all services during scanning (filtering happens in callback)
		assert.Len(t, webContainer.DependsOn, 3)

		// Verify all dependencies are present
		depNames := make([]string, len(webContainer.DependsOn))
		for i, dep := range webContainer.DependsOn {
			depNames[i] = dep.Name
		}
		assert.Contains(t, depNames, "api")
		assert.Contains(t, depNames, "db")
		assert.Contains(t, depNames, "cache")

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

	t.Run("handles multiple identical build sections", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with multiple identical build services
		composeContent := `version: '3'
services:
  web:
    build: .
    ports:
      - "8080:8080"
  api:
    build: .
  worker:
    image: nginx:latest
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner - should succeed
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Should have 3 containers
		assert.Len(t, srcInfo.Containers, 3)

		// Container field should be set to the first build service
		assert.Equal(t, "web", srcInfo.Container)

		// BuildContainers should include both build services
		assert.Equal(t, []string{"web", "api"}, srcInfo.BuildContainers)

		// Verify containers
		var webContainer, apiContainer, workerContainer *Container
		for i := range srcInfo.Containers {
			switch srcInfo.Containers[i].Name {
			case "web":
				webContainer = &srcInfo.Containers[i]
			case "api":
				apiContainer = &srcInfo.Containers[i]
			case "worker":
				workerContainer = &srcInfo.Containers[i]
			}
		}
		require.NotNil(t, webContainer)
		require.NotNil(t, apiContainer)
		require.NotNil(t, workerContainer)

		// Both web and api containers should have build context
		assert.Equal(t, tmpDir, webContainer.BuildContext)
		assert.Equal(t, "", webContainer.Image)
		assert.Equal(t, tmpDir, apiContainer.BuildContext)
		assert.Equal(t, "", apiContainer.Image)

		// Worker container should have image
		assert.Equal(t, "nginx:latest", workerContainer.Image)
		assert.Equal(t, "", workerContainer.BuildContext)
	})

	t.Run("errors on multiple different build sections", func(t *testing.T) {
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
		assert.Contains(t, err.Error(), "multiple services with different build configurations found")
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

		// Verify all containers are returned during scanning
		assert.Len(t, srcInfo.Containers, 3)

		// Find the web container
		var webContainer *Container
		for i := range srcInfo.Containers {
			if srcInfo.Containers[i].Name == "web" {
				webContainer = &srcInfo.Containers[i]
				break
			}
		}
		require.NotNil(t, webContainer)

		// Verify non-database env vars remain in container environment
		assert.Equal(t, "not-a-database-key", webContainer.Env["API_KEY"])
		assert.Equal(t, "value", webContainer.Env["NORMAL_ENV"])

		// Verify database env vars were removed from container environment
		assert.NotContains(t, webContainer.Env, "DATABASE_URL")
		assert.NotContains(t, webContainer.Env, "REDIS_URL")
		assert.NotContains(t, webContainer.Env, "DB_PASSWORD")

		// Verify container has the secrets it needs access to
		assert.Len(t, webContainer.Secrets, 3) // DB_PASSWORD + DATABASE_URL + REDIS_URL
		assert.Contains(t, webContainer.Secrets, "DB_PASSWORD")
		// DATABASE_URL and REDIS_URL are in secrets list (even though Fly provides them, container needs access)
		assert.Contains(t, webContainer.Secrets, "DATABASE_URL")
		assert.Contains(t, webContainer.Secrets, "REDIS_URL")

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

	t.Run("handles docker compose secrets section", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a master.key file
		masterKeyPath := filepath.Join(tmpDir, "master.key")
		err = os.WriteFile(masterKeyPath, []byte("secret-master-key-value\n"), 0600)
		require.NoError(t, err)

		// Create a docker-compose.yml with secrets section
		composeContent := `version: '3.8'
services:
  web:
    image: myapp:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://user:pass@postgres-db/mydb
    secrets:
      - master_key
      - source: api_key
        target: /run/secrets/api_key
  postgres-db:
    image: postgres:13
    environment:
      - POSTGRES_PASSWORD=dbpass

secrets:
  master_key:
    file: ./master.key
  api_key:
    external: true
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify Docker Compose secrets were extracted
		// Should have master_key (api_key is external so it's skipped)
		secretKeys := make(map[string]string)
		for _, secret := range srcInfo.Secrets {
			secretKeys[secret.Key] = secret.Value
		}

		// Check master_key was read from file
		assert.Contains(t, secretKeys, "master_key")
		assert.Equal(t, "secret-master-key-value", secretKeys["master_key"])

		// api_key should not be in secrets (it's external)
		assert.NotContains(t, secretKeys, "api_key")

		// Verify all containers are returned during scanning
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
		assert.Contains(t, webContainer.Secrets, "master_key")
		assert.Contains(t, webContainer.Secrets, "api_key")

		// DATABASE_URL should be in container secrets (Fly managed db detected)
		assert.Contains(t, webContainer.Secrets, "DATABASE_URL")

		// DATABASE_URL should have been removed from environment
		assert.NotContains(t, webContainer.Env, "DATABASE_URL")
	})

	t.Run("database services get DATABASE_URL secret access", func(t *testing.T) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "docker-compose-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		// Create a docker-compose.yml with database service
		composeContent := `version: '3.8'
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
    environment:
      - DATABASE_URL=postgres://root:password@postgres-db/
      - API_KEY=somekey
  postgres-db:
    image: postgres
    environment:
      - POSTGRES_PASSWORD=password
`
		err = os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644)
		require.NoError(t, err)

		// Run scanner
		srcInfo, err := configureDockerCompose(tmpDir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, srcInfo)

		// Verify Postgres was detected
		assert.Equal(t, DatabaseKindPostgres, srcInfo.DatabaseDesired)

		// Verify all containers are returned during scanning
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

		// DATABASE_URL should be in container's secrets list (needed for Fly-provided secret)
		assert.Contains(t, webContainer.Secrets, "DATABASE_URL")

		// DATABASE_URL should NOT be in the global secrets (Fly provides it)
		for _, secret := range srcInfo.Secrets {
			assert.NotEqual(t, "DATABASE_URL", secret.Key)
		}

		// Regular env vars should remain
		assert.Equal(t, "somekey", webContainer.Env["API_KEY"])
	})
}

func TestParseBindMount(t *testing.T) {
	t.Run("parses simple bind mount", func(t *testing.T) {
		result := parseBindMount("./nginx.conf:/etc/nginx/conf.d/default.conf", "/app")
		require.NotNil(t, result)
		assert.Equal(t, "/etc/nginx/conf.d/default.conf", result.GuestPath)
		assert.Equal(t, "nginx.conf", result.LocalPath)
		assert.Equal(t, 0644, result.Mode)
	})

	t.Run("parses read-only bind mount", func(t *testing.T) {
		result := parseBindMount("./nginx.conf:/etc/nginx/conf.d/default.conf:ro", "/app")
		require.NotNil(t, result)
		assert.Equal(t, "/etc/nginx/conf.d/default.conf", result.GuestPath)
		assert.Equal(t, "nginx.conf", result.LocalPath)
		assert.Equal(t, 0444, result.Mode)
	})

	t.Run("skips named volumes", func(t *testing.T) {
		result := parseBindMount("data:/app/data", "/app")
		assert.Nil(t, result)
	})

	t.Run("skips volumes without colon", func(t *testing.T) {
		result := parseBindMount("data", "/app")
		assert.Nil(t, result)
	})

	t.Run("handles absolute paths", func(t *testing.T) {
		result := parseBindMount("/host/config:/app/config", "/app")
		require.NotNil(t, result)
		assert.Equal(t, "/app/config", result.GuestPath)
		assert.Equal(t, "config", result.LocalPath) // Should use basename for absolute paths
		assert.Equal(t, 0644, result.Mode)
	})
}
