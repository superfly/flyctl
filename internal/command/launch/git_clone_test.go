package launch

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
