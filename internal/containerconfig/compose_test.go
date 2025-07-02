package containerconfig

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fly "github.com/superfly/fly-go"
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
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	// Verify the parsed configuration - now always uses containers
	// Main image should be empty when using containers
	if mConfig.Image != "" {
		t.Errorf("Expected main image to be empty, got '%s'", mConfig.Image)
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
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err != nil {
		t.Fatalf("Failed to parse multi-service compose file: %v", err)
	}

	// Verify the main image is empty when using containers
	if mConfig.Image != "" {
		t.Errorf("Expected main image to be empty, got '%s'", mConfig.Image)
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
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	// Main image should be empty when using containers
	if mConfig.Image != "" {
		t.Errorf("Expected main image to be empty, got '%s'", mConfig.Image)
	}

	// Verify containers were created
	if len(mConfig.Containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(mConfig.Containers))
	}
}

func TestComposeVolumeAndHealthcheck(t *testing.T) {
	// Create a compose file with volumes and health checks
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	// Copy nginx.conf to temp directory
	nginxConf := `server {
    listen 80;
    location / {
        proxy_pass http://echo:80;
    }
}`
	nginxPath := filepath.Join(tmpDir, "nginx.conf")
	if err := os.WriteFile(nginxPath, []byte(nginxConf), 0644); err != nil {
		t.Fatalf("Failed to write nginx.conf: %v", err)
	}

	composeContent := `version: "3.8"
services:
  nginx:
    image: nginx:latest
    volumes:
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
  echo:
    image: ealen/echo-server
    healthcheck:
      test: ["CMD", "wget", "--spider", "localhost:80"]
      interval: 30s
      timeout: 10s
      retries: 3
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse the compose file
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	// Find the nginx container
	var nginxContainer *fly.ContainerConfig
	var echoContainer *fly.ContainerConfig
	for _, container := range mConfig.Containers {
		if container.Name == "nginx" {
			nginxContainer = container
		} else if container.Name == "echo" {
			echoContainer = container
		}
	}

	if nginxContainer == nil {
		t.Fatal("nginx container not found")
	}
	if echoContainer == nil {
		t.Fatal("echo container not found")
	}

	// Check nginx has the volume mounted
	nginxConfFound := false
	for _, file := range nginxContainer.Files {
		if file.GuestPath == "/etc/nginx/conf.d/default.conf" {
			nginxConfFound = true
			// Check it's read-only
			if file.Mode != 0444 {
				t.Errorf("Expected nginx.conf to be read-only (0444), got %o", file.Mode)
			}
			// Check content
			if file.RawValue != nil {
				decoded, err := base64.StdEncoding.DecodeString(*file.RawValue)
				if err != nil {
					t.Errorf("Failed to decode nginx.conf content: %v", err)
				} else if !strings.Contains(string(decoded), "proxy_pass http://echo:80") {
					t.Errorf("nginx.conf should contain proxy_pass directive")
				}
			}
			break
		}
	}
	if !nginxConfFound {
		t.Error("nginx.conf volume mount not found")
	}

	// Check echo container has health check
	if len(echoContainer.Healthchecks) == 0 {
		t.Error("echo container should have health check")
	} else {
		hc := echoContainer.Healthchecks[0]
		if hc.Exec == nil {
			t.Error("Expected exec health check")
		} else {
			// Command should be ["wget", "--spider", "localhost:80"] (without CMD)
			expectedCmd := []string{"wget", "--spider", "localhost:80"}
			if len(hc.Exec.Command) != len(expectedCmd) {
				t.Errorf("Expected health check command %v, got %v", expectedCmd, hc.Exec.Command)
			} else {
				for i, cmd := range expectedCmd {
					if i < len(hc.Exec.Command) && hc.Exec.Command[i] != cmd {
						t.Errorf("Expected health check command[%d] '%s', got '%s'", i, cmd, hc.Exec.Command[i])
					}
				}
			}
		}
		// Check intervals
		if hc.Interval != 30 {
			t.Errorf("Expected interval 30, got %d", hc.Interval)
		}
		if hc.Timeout != 10 {
			t.Errorf("Expected timeout 10, got %d", hc.Timeout)
		}
		if hc.FailureThreshold != 3 {
			t.Errorf("Expected failure threshold 3, got %d", hc.FailureThreshold)
		}
	}
}

func TestParseComposeFileWithBuild(t *testing.T) {
	// Create a compose file with build section
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  app:
    build: .
    environment:
      APP_ENV: production
  db:
    image: postgres:14
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse the compose file
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err != nil {
		t.Fatalf("Failed to parse compose file with build: %v", err)
	}

	// Verify containers were created
	if len(mConfig.Containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(mConfig.Containers))
	}

	// Find containers
	var appContainer, dbContainer *fly.ContainerConfig
	for _, container := range mConfig.Containers {
		switch container.Name {
		case "app":
			appContainer = container
		case "db":
			dbContainer = container
		}
	}

	if appContainer == nil {
		t.Fatal("app container not found")
	}
	if dbContainer == nil {
		t.Fatal("db container not found")
	}

	// Service with build should have image "."
	if appContainer.Image != "." {
		t.Errorf("Expected app container image '.', got '%s'", appContainer.Image)
	}

	// Service without build should have its specified image
	if dbContainer.Image != "postgres:14" {
		t.Errorf("Expected db container image 'postgres:14', got '%s'", dbContainer.Image)
	}
}

func TestParseComposeFileMultipleBuildError(t *testing.T) {
	// Create a compose file with multiple build sections
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  app1:
    build: .
  app2:
    build:
      context: ./app2
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse should fail
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err == nil {
		t.Fatal("Expected error for multiple services with build, got nil")
	}

	if !strings.Contains(err.Error(), "only one service can specify build") {
		t.Errorf("Expected error about multiple build services, got: %v", err)
	}
}

func TestParseComposeFileMissingImageAndBuild(t *testing.T) {
	// Create a compose file with a service that has neither image nor build
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  app:
    environment:
      APP_ENV: production
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse should fail
	mConfig := &fly.MachineConfig{}
	err := ParseComposeFile(mConfig, composePath)
	if err == nil {
		t.Fatal("Expected error for service without image or build, got nil")
	}

	if !strings.Contains(err.Error(), "must specify either 'image' or 'build'") {
		t.Errorf("Expected error about missing image or build, got: %v", err)
	}
}
