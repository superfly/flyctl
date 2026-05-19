package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

func newTestConfig() *config.Config {
	return &config.Config{
		RegistryHost: "registry.fly.io",
		Tokens:       tokens.Parse("fm2_test-macaroon"),
	}
}

func TestConfigureDockerJSON_writesConfigAt0600_freshInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("configureDockerJSON is unsupported on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := configureDockerJSON(newTestConfig()); err != nil {
		t.Fatalf("configureDockerJSON: %v", err)
	}

	configPath := filepath.Join(home, ".docker", "config.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config.json: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("config.json mode = %o, want %o", got, want)
	}
}

func TestConfigureDockerJSON_appliesPermsToExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("configureDockerJSON is unsupported on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	dockerDir := filepath.Join(home, ".docker")
	if err := os.Mkdir(dockerDir, 0o755); err != nil {
		t.Fatalf("mkdir .docker: %v", err)
	}
	configPath := filepath.Join(dockerDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"auths":{}}`), 0o644); err != nil {
		t.Fatalf("seed config.json: %v", err)
	}

	if err := configureDockerJSON(newTestConfig()); err != nil {
		t.Fatalf("configureDockerJSON: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config.json: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("config.json mode = %o, want %o (existing-file rewrite path)", got, want)
	}
}

func TestEnsureDockerConfigDir_createsDirAt0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits don't translate to windows")
	}
	home := t.TempDir()

	if err := ensureDockerConfigDir(home); err != nil {
		t.Fatalf("ensureDockerConfigDir: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".docker"))
	if err != nil {
		t.Fatalf("stat .docker: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf(".docker is not a directory")
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Errorf(".docker mode = %o, want %o", got, want)
	}
}

func TestEnsureDockerConfigDir_leavesExistingDirAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits don't translate to windows")
	}
	home := t.TempDir()
	dockerDir := filepath.Join(home, ".docker")
	if err := os.Mkdir(dockerDir, 0o755); err != nil {
		t.Fatalf("mkdir .docker: %v", err)
	}

	if err := ensureDockerConfigDir(home); err != nil {
		t.Fatalf("ensureDockerConfigDir: %v", err)
	}

	info, err := os.Stat(dockerDir)
	if err != nil {
		t.Fatalf("stat .docker: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Errorf(".docker mode = %o, want %o (must not retouch user-owned dirs)", got, want)
	}
}

func TestAddFlyAuthToDockerConfig_preservesOtherRegistries(t *testing.T) {
	existing := []byte(`{
		"auths": {
			"ghcr.io": {"auth": "Z2hjci10b2tlbg=="}
		},
		"credsStore": "osxkeychain"
	}`)

	out, err := addFlyAuthToDockerConfig(newTestConfig(), existing)
	if err != nil {
		t.Fatalf("addFlyAuthToDockerConfig: %v", err)
	}

	var parsed struct {
		Auths      map[string]map[string]string `json:"auths"`
		CredsStore string                       `json:"credsStore"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v\n%s", err, out)
	}

	if _, ok := parsed.Auths["ghcr.io"]; !ok {
		t.Errorf("ghcr.io auth was dropped; result: %s", out)
	}
	if _, ok := parsed.Auths["registry.fly.io"]; !ok {
		t.Errorf("registry.fly.io auth was not added; result: %s", out)
	}
	if parsed.CredsStore != "osxkeychain" {
		t.Errorf("credsStore field was dropped: %q", parsed.CredsStore)
	}
}
