package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superfly/flyctl/internal/appconfig"
)

// TestIsURL tests the isURL function
func TestIsURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://example.com/dockerfile", true},
		{"http://example.com/dockerfile", true},
		{"file:///path/to/dockerfile", false},
		{"./dockerfile", false},
		{"/absolute/path/dockerfile", false},
		{"relative/path/dockerfile", false},
		{"", false},
	}

	for _, test := range tests {
		result := isURL(test.input)
		if result != test.expected {
			t.Errorf("isURL(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

// TestDownloadFile tests downloading a file from a URL
func TestDownloadFile(t *testing.T) {
	// Create a test server that serves a mock Dockerfile
	mockDockerfile := "FROM alpine:latest\nRUN echo 'Hello, World!'\nCMD [\"echo\", \"test\"]"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockDockerfile))
	}))
	defer server.Close()

	ctx := context.Background()
	tmpFile, err := downloadFile(ctx, server.URL)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}
	defer os.Remove(tmpFile) // Clean up

	// Verify the file was created and has correct content
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(content) != mockDockerfile {
		t.Errorf("Downloaded content mismatch:\nExpected: %q\nGot: %q", mockDockerfile, string(content))
	}

	// Verify the file follows our naming pattern
	if !strings.Contains(filepath.Base(tmpFile), "dockerfile-") || !strings.HasSuffix(tmpFile, ".tmp") {
		t.Errorf("Downloaded file name doesn't match expected pattern: %s", tmpFile)
	}
}

// TestResolveDockerfilePathWithURL tests the resolveDockerfilePath function with a URL
func TestResolveDockerfilePathWithURL(t *testing.T) {
	// Create a test server that serves a mock Dockerfile
	mockDockerfile := "FROM alpine:latest\nRUN echo 'Test Dockerfile from URL'\nCMD [\"echo\", \"success\"]"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockDockerfile))
	}))
	defer server.Close()

	// Create a temporary fly.toml file
	tmpDir, err := os.MkdirTemp("", "flyctl-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "fly.toml")
	configContent := `app = "test-app"
primary_region = "ams"

[build]
dockerfile = "` + server.URL + `"
`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load the app config
	config, err := appconfig.LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	resolvedPath, err := resolveDockerfilePath(ctx, config)
	if err != nil {
		t.Fatalf("resolveDockerfilePath failed: %v", err)
	}
	defer os.Remove(resolvedPath) // Clean up

	// Verify the path is not empty and points to a real file
	if resolvedPath == "" {
		t.Error("resolveDockerfilePath returned empty path")
	}

	// Verify the file exists and has correct content
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		t.Fatalf("Failed to read resolved Dockerfile: %v", err)
	}

	if string(content) != mockDockerfile {
		t.Errorf("Resolved Dockerfile content mismatch:\nExpected: %q\nGot: %q", mockDockerfile, string(content))
	}

	// Verify it's a temporary file
	if !strings.Contains(filepath.Base(resolvedPath), "dockerfile-") || !strings.HasSuffix(resolvedPath, ".tmp") {
		t.Errorf("Resolved path doesn't match expected temporary file pattern: %s", resolvedPath)
	}
}

// TestResolveDockerfilePathWithLocalFile tests the resolveDockerfilePath function with a local file
func TestResolveDockerfilePathWithLocalFile(t *testing.T) {
	// Create a temporary directory with a Dockerfile
	tmpDir, err := os.MkdirTemp("", "flyctl-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	dockerfileContent := "FROM alpine:latest\nRUN echo 'Local Dockerfile'\nCMD [\"echo\", \"local\"]"
	err = os.WriteFile(dockerfile, []byte(dockerfileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write Dockerfile: %v", err)
	}

	configFile := filepath.Join(tmpDir, "fly.toml")
	configContent := `app = "test-app"
primary_region = "ams"

[build]
dockerfile = "Dockerfile"
`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load the app config
	config, err := appconfig.LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	resolvedPath, err := resolveDockerfilePath(ctx, config)
	if err != nil {
		t.Fatalf("resolveDockerfilePath failed: %v", err)
	}

	// Verify the path points to our local Dockerfile
	expectedPath, _ := filepath.Abs(dockerfile)
	if resolvedPath != expectedPath {
		t.Errorf("Expected resolved path %q, got %q", expectedPath, resolvedPath)
	}

	// Verify the file exists and has correct content
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		t.Fatalf("Failed to read resolved Dockerfile: %v", err)
	}

	if string(content) != dockerfileContent {
		t.Errorf("Resolved Dockerfile content mismatch:\nExpected: %q\nGot: %q", dockerfileContent, string(content))
	}
}
