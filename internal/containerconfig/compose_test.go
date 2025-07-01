package containerconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseComposeFile(t *testing.T) {
	// Create a temporary compose file for testing
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    environment:
      ENV_VAR: value
    restart: always
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse the compose file
	mConfig, err := ParseComposeFile(composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	// Verify the parsed configuration - now always uses containers
	if mConfig.Image != "nginx:latest" {
		t.Errorf("Expected main image 'nginx:latest', got '%s'", mConfig.Image)
	}

	// Should have one container
	if len(mConfig.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(mConfig.Containers))
	}

	// Check the container details
	container := mConfig.Containers[0]
	if container.Name != "web" {
		t.Errorf("Expected container name 'web', got '%s'", container.Name)
	}

	if container.Image != "nginx:latest" {
		t.Errorf("Expected container image 'nginx:latest', got '%s'", container.Image)
	}

	if container.ExtraEnv["ENV_VAR"] != "value" {
		t.Errorf("Expected ENV_VAR='value', got '%s'", container.ExtraEnv["ENV_VAR"])
	}
}

func TestParseComposeFileMultiService(t *testing.T) {
	// Create a temporary compose file with multiple services
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  app:
    image: myapp:latest
    environment:
      APP_ENV: production
    command: ["./start.sh"]
  db:
    image: postgres:14
    environment:
      POSTGRES_PASSWORD: secret
  cache:
    image: redis:alpine
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse the compose file - should succeed with containers
	mConfig, err := ParseComposeFile(composePath)
	if err != nil {
		t.Fatalf("Failed to parse multi-service compose file: %v", err)
	}

	// Verify the main image is set (should be "app" service)
	if mConfig.Image != "myapp:latest" {
		t.Errorf("Expected main image 'myapp:latest', got '%s'", mConfig.Image)
	}

	// Verify containers were created
	if len(mConfig.Containers) != 3 {
		t.Errorf("Expected 3 containers, got %d", len(mConfig.Containers))
	}

	// Check container details
	containerNames := make(map[string]bool)
	for _, container := range mConfig.Containers {
		containerNames[container.Name] = true

		switch container.Name {
		case "app":
			if container.Image != "myapp:latest" {
				t.Errorf("Expected app container image 'myapp:latest', got '%s'", container.Image)
			}
			if container.ExtraEnv["APP_ENV"] != "production" {
				t.Errorf("Expected APP_ENV='production', got '%s'", container.ExtraEnv["APP_ENV"])
			}
			if len(container.CmdOverride) == 0 || container.CmdOverride[0] != "./start.sh" {
				t.Errorf("Expected command './start.sh', got %v", container.CmdOverride)
			}
		case "db":
			if container.Image != "postgres:14" {
				t.Errorf("Expected db container image 'postgres:14', got '%s'", container.Image)
			}
			if container.ExtraEnv["POSTGRES_PASSWORD"] != "secret" {
				t.Errorf("Expected POSTGRES_PASSWORD='secret', got '%s'", container.ExtraEnv["POSTGRES_PASSWORD"])
			}
		case "cache":
			if container.Image != "redis:alpine" {
				t.Errorf("Expected cache container image 'redis:alpine', got '%s'", container.Image)
			}
		}
	}

	// Verify all expected containers exist
	for _, name := range []string{"app", "db", "cache"} {
		if !containerNames[name] {
			t.Errorf("Expected container '%s' not found", name)
		}
	}
}

func TestParseComposeFileMultiServiceNoApp(t *testing.T) {
	// Create a compose file without an "app" service
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  web:
    image: nginx:latest
  backend:
    image: api:latest
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse the compose file
	mConfig, err := ParseComposeFile(composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	// When no "app" service exists, the first service should be used as main
	// Note: map iteration order is not guaranteed, so we just check that an image is set
	if mConfig.Image == "" {
		t.Error("Expected main image to be set from one of the services")
	}

	// Verify containers were created
	if len(mConfig.Containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(mConfig.Containers))
	}
}
