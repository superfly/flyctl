package launch

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envValue(cmd *exec.Cmd, key string) (string, bool) {
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}

	value, found := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value, found = v, true // last one wins, as in exec
		}
	}

	return value, found
}

func TestGitCloneCmdArgs(t *testing.T) {
	cmd := gitCloneCmd(true, "https://example.com/repo.git", "dest")

	assert.Equal(t, []string{"git", "clone", "--recurse-submodules", "https://example.com/repo.git", "dest"}, cmd.Args)
	assert.Equal(t, os.Stdin, cmd.Stdin)
	assert.Equal(t, os.Stdout, cmd.Stdout)
	assert.Equal(t, os.Stderr, cmd.Stderr)
}

func TestGitCloneCmdInteractiveLeavesEnvAlone(t *testing.T) {
	cmd := gitCloneCmd(true, "repo", "dest")

	assert.Nil(t, cmd.Env, "an interactive clone inherits flyctl's environment unchanged")
}

func TestGitCloneCmdNonInteractiveDisablesGitPrompts(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "")
	os.Unsetenv("GIT_TERMINAL_PROMPT")

	cmd := gitCloneCmd(false, "repo", "dest")

	v, ok := envValue(cmd, "GIT_TERMINAL_PROMPT")
	assert.True(t, ok)
	assert.Equal(t, "0", v)
	_, hasPath := envValue(cmd, "PATH")
	assert.True(t, hasPath, "the rest of the environment is kept")
}

func TestGitCloneCmdNonInteractiveKeepsCallersSetting(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1")

	cmd := gitCloneCmd(false, "repo", "dest")

	v, _ := envValue(cmd, "GIT_TERMINAL_PROMPT")
	assert.Equal(t, "1", v)
}

func TestGitCloneCmdNonInteractiveKeepsCallersEmptySetting(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "")

	cmd := gitCloneCmd(false, "repo", "dest")

	assert.Nil(t, cmd.Env, "a GIT_TERMINAL_PROMPT the caller set, even to empty, is left alone")
}

// Runs a real `git clone` against a local server that asks for credentials,
// and checks git gives up because prompts are disabled rather than asking.
func TestGitCloneCmdNonInteractiveFailsFastOnAuth(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// Keep the user's and the system's git config, credential helpers and
	// askpass programs out of it, so git's only way to get credentials is the
	// terminal.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, k := range []string{"GIT_TERMINAL_PROMPT", "GIT_ASKPASS", "SSH_ASKPASS"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	cmd := gitCloneCmd(false, srv.URL+"/private.git", filepath.Join(t.TempDir(), "dest"))
	var stderr bytes.Buffer
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, stderr.String(), "terminal prompts disabled", "git should refuse to prompt, not fail for some other reason")
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("git clone is waiting for credentials")
	}
}
