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

	// Verify the parsed configuration
	if mConfig.Image != "nginx:latest" {
		t.Errorf("Expected image 'nginx:latest', got '%s'", mConfig.Image)
	}

	if mConfig.Env["ENV_VAR"] != "value" {
		t.Errorf("Expected ENV_VAR='value', got '%s'", mConfig.Env["ENV_VAR"])
	}

	if len(mConfig.Services) == 0 {
		t.Error("Expected services to be defined")
	}

	if mConfig.Restart.Policy != "always" {
		t.Errorf("Expected restart policy 'always', got '%s'", mConfig.Restart.Policy)
	}
}

func TestParseComposeFileMultiService(t *testing.T) {
	// Create a temporary compose file with multiple services
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "compose.yml")

	composeContent := `version: "3"
services:
  web:
    image: nginx:latest
  db:
    image: postgres:latest
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write test compose file: %v", err)
	}

	// Parse the compose file - should fail for multi-service
	_, err := ParseComposeFile(composePath)
	if err == nil {
		t.Fatal("Expected error for multi-service compose file, got nil")
	}
}
