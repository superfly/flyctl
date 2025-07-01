package containerconfig

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestComposeServiceNetworking(t *testing.T) {
	// Create a compose file with multiple services for networking test
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  web:
    image: nginx:alpine
    environment:
      DB_HOST: db
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

	// Parse the compose file
	mConfig, err := ParseComposeFile(composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	// Verify all containers have the hosts script
	for _, container := range mConfig.Containers {
		// Check that hosts script file is present
		if len(container.Files) == 0 {
			t.Errorf("Container '%s' should have hosts script file", container.Name)
			continue
		}

		hostsFile := container.Files[0]
		if hostsFile.GuestPath != "/usr/local/bin/hosts-update.sh" {
			t.Errorf("Expected hosts script at '/usr/local/bin/hosts-update.sh', got '%s'", hostsFile.GuestPath)
		}

		if hostsFile.Mode != 0755 {
			t.Errorf("Expected hosts script to be executable (0755), got %o", hostsFile.Mode)
		}

		// Check that entrypoint override includes the hosts script
		if len(container.EntrypointOverride) == 0 || container.EntrypointOverride[0] != "/usr/local/bin/hosts-update.sh" {
			t.Errorf("Container '%s' should have hosts script as first entrypoint, got %v", container.Name, container.EntrypointOverride)
		}
	}

	// Verify the hosts script content includes all service names
	if len(mConfig.Containers) > 0 {
		hostsFile := mConfig.Containers[0].Files[0]
		if hostsFile.RawValue == nil {
			t.Fatal("Hosts script content should not be nil")
		}

		// Decode the base64 content
		scriptContent, err := base64.StdEncoding.DecodeString(*hostsFile.RawValue)
		if err != nil {
			t.Fatalf("Failed to decode hosts script: %v", err)
		}

		scriptStr := string(scriptContent)

		// Check that all service names are mapped to localhost
		for _, serviceName := range []string{"web", "db", "cache"} {
			expectedLine := fmt.Sprintf("127.0.0.1 %s", serviceName)
			if !strings.Contains(scriptStr, expectedLine) {
				t.Errorf("Hosts script should contain '%s', got:\n%s", expectedLine, scriptStr)
			}
		}
	}
}

func TestExtractImageEntrypoint(t *testing.T) {
	// Skip this test if we can't access the registry (e.g., in CI without credentials)
	t.Skip("Skipping image entrypoint extraction test - requires registry access")

	// If you want to run this test locally, remove the Skip above
	tests := []struct {
		image    string
		hasEntry bool // Whether we expect an entrypoint
	}{
		{"nginx:alpine", true},
		{"nginx:latest", true},
		{"postgres:14", true},
		{"redis:latest", true},
		{"alpine:latest", false}, // Alpine usually has no entrypoint, just CMD
		{"ubuntu:20.04", false},  // Ubuntu usually has no entrypoint
		{"busybox:latest", false},
	}

	for _, test := range tests {
		result, err := extractImageEntrypoint(test.image)
		if err != nil {
			// It's okay if we can't fetch some images (private, rate limits, etc.)
			t.Logf("Could not fetch config for %s: %v", test.image, err)
			continue
		}

		if test.hasEntry && len(result) == 0 {
			t.Errorf("Expected image '%s' to have an entrypoint, but got none", test.image)
		} else if !test.hasEntry && len(result) > 0 {
			t.Errorf("Expected image '%s' to have no entrypoint, but got %v", test.image, result)
		}

		if len(result) > 0 {
			t.Logf("Image %s has entrypoint: %v", test.image, result)
		}
	}
}
