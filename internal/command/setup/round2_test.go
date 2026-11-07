package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "flyctl-setup-state-")
	if err != nil {
		panic(err)
	}
	prev, had := os.LookupEnv("FLY_CONFIG_DIR")
	if err := os.Setenv("FLY_CONFIG_DIR", filepath.Join(dir, ".fly")); err != nil {
		panic(err)
	}
	// Direct writer tests must not use the invoking user's client configs.
	// Individual override tests opt in with t.Setenv.
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "COPILOT_HOME", "XDG_CONFIG_HOME"} {
		if err := os.Unsetenv(key); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	if had {
		_ = os.Setenv("FLY_CONFIG_DIR", prev)
	} else {
		_ = os.Unsetenv("FLY_CONFIG_DIR")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// Windows can expose temporary directories through short-name aliases. Tests
// that assert resolved paths must compare against the canonical directory.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestSymlinkToSSHIsRefusedEvenWithForce(t *testing.T) {
	home := canonicalTempDir(t)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	keys := filepath.Join(home, ".ssh", "authorized_keys")
	original := []byte("ssh-ed25519 AAAA keep\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(keys), 0o700))
	require.NoError(t, os.WriteFile(keys, original, 0o600))
	link := filepath.Join(home, ".claude.json")
	if err := os.Symlink(keys, link); err != nil {
		t.Skip(err)
	}

	for _, force := range []bool{false, true} {
		out := configureMCP(home, "claude-code", force, false)
		require.Equal(t, statusFailed, out.Status, out.Detail)
		require.Error(t, out.Err)
		require.Contains(t, out.Err.Error(), link)
		require.Contains(t, out.Err.Error(), keys)
		require.Equal(t, original, readFile(t, keys))
		info, err := os.Lstat(link)
		require.NoError(t, err)
		require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
	}
}

func TestSymlinkedCursorOutsideHomeIsRefused(t *testing.T) {
	home := canonicalTempDir(t)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	outside := canonicalTempDir(t)
	link := filepath.Join(home, ".cursor")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}

	logical := filepath.Join(link, "mcp.json")
	out := configureMCP(home, "cursor", true, false)
	require.Equal(t, statusFailed, out.Status, out.Detail)
	require.Error(t, out.Err)
	require.Contains(t, out.Err.Error(), logical)
	require.Contains(t, out.Err.Error(), filepath.Join(outside, "mcp.json"))
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
}

func TestSymlinkOutsideHomeIsRefused(t *testing.T) {
	home := canonicalTempDir(t)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	outside := filepath.Join(canonicalTempDir(t), "passwd")
	original := []byte("root:x:0:0\n")
	require.NoError(t, os.WriteFile(outside, original, 0o644))
	link := filepath.Join(home, ".claude.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "claude-code", true, false)
	require.Equal(t, statusFailed, out.Status, out.Detail)
	require.Contains(t, out.Err.Error(), link)
	require.Contains(t, out.Err.Error(), outside)
	require.Equal(t, original, readFile(t, outside))
}

func TestDanglingParentSymlinkDoesNotCreateOutside(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	outside := filepath.Join(t.TempDir(), "missing-cursor")
	link := filepath.Join(home, ".cursor")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "cursor", false, false)
	require.Equal(t, statusFailed, out.Status, out.Detail)
	require.Contains(t, out.Err.Error(), "symlink")
	_, err := os.Stat(outside)
	require.True(t, os.IsNotExist(err))
}

func TestExplicitConfigDirOutsideHomeIsAllowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	dest := t.TempDir()
	t.Setenv("CODEX_HOME", dest)

	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.FileExists(t, filepath.Join(dest, "config.toml"))
}

func TestSymlinkOutOfExplicitConfigDirIsRefused(t *testing.T) {
	home := canonicalTempDir(t)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	dest := canonicalTempDir(t)
	outside := filepath.Join(canonicalTempDir(t), "config.toml")
	original := []byte("url = \"https://example.com\"\n")
	require.NoError(t, os.WriteFile(outside, original, 0o644))
	link := filepath.Join(dest, "config.toml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	t.Setenv("CODEX_HOME", dest)

	out := configureMCP(home, "codex", true, false)
	require.Equal(t, statusFailed, out.Status, out.Detail)
	require.Contains(t, out.Err.Error(), link)
	require.Contains(t, out.Err.Error(), outside)
	require.Equal(t, original, readFile(t, outside))
}

func TestResolveOnceDoesNotFollowASwappedSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	real := filepath.Join(home, "real-claude.json")
	writeExisting(t, real, []byte(`{"theme":"dark"}`), 0o644)
	keys := filepath.Join(home, ".ssh", "authorized_keys")
	original := []byte("ssh-ed25519 AAAA keep\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(keys), 0o700))
	require.NoError(t, os.WriteFile(keys, original, 0o600))
	link := filepath.Join(home, ".claude.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}

	beforeWriteHook = func() {
		require.NoError(t, os.Remove(link))
		require.NoError(t, os.Symlink(keys, link))
	}
	t.Cleanup(func() { beforeWriteHook = nil })

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.Contains(t, string(readFile(t, real)), hostedMCPURL)
	require.Equal(t, original, readFile(t, keys))
}

func TestPlantedConfigLockSymlinkIsNotFollowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	path := filepath.Join(home, ".claude.json")
	leaked := filepath.Join(home, "created-elsewhere")
	link := path + ".lock"
	if err := os.Symlink(leaked, link); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	_, err := os.Stat(leaked)
	require.True(t, os.IsNotExist(err))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
	_, err = os.Lstat(path + ".lock")
	require.NoError(t, err)
}

func TestStateDirLockSymlinkIsRefused(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	path := filepath.Join(home, ".claude.json")
	lockPath, err := configLockPath(path)
	require.NoError(t, err)
	leaked := filepath.Join(home, "leaked-lock-target")
	if err := os.Symlink(leaked, lockPath); err != nil {
		t.Skip(err)
	}

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusFailed, out.Status, out.Detail)
	require.Contains(t, out.Err.Error(), "symlink")
	require.Contains(t, out.Err.Error(), lockPath)
	_, err = os.Stat(leaked)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestOpenLockFileDoesNotFollowSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("O_NOFOLLOW is enforced on unix")
	}
	dir := t.TempDir()
	leaked := filepath.Join(dir, "leaked")
	link := filepath.Join(dir, "lock")
	if err := os.Symlink(leaked, link); err != nil {
		t.Skip(err)
	}

	f, err := openLockFile(link)
	if f != nil {
		require.NoError(t, f.Close())
	}
	require.Error(t, err)
	_, statErr := os.Stat(leaked)
	require.True(t, os.IsNotExist(statErr))
}

func TestCodexDisabledEntryIsAConflictUntilForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("[mcp_servers.fly]\nurl = \"https://mcp.fly.io\"\nenabled = false # off\nnote = \"keep-me\"\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusConflict, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, path))

	out = configureMCP(home, "codex", true, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	body := string(readFile(t, path))
	require.Contains(t, body, "enabled = true")
	require.Contains(t, body, "# off")
	require.Contains(t, body, "note = \"keep-me\"")
	require.NotContains(t, body, "enabled = false")
	assertNoSecrets(t, readFile(t, path))

	out = configureMCP(home, "codex", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
}

func TestOpenCodeJSONCIsEditedWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	dir := filepath.Join(home, ".config", "opencode")
	jsonc := filepath.Join(dir, "opencode.jsonc")
	jsonPath := filepath.Join(dir, "opencode.json")
	writeExisting(t, jsonc, []byte("{\n  // keep\n  \"mcp\": {}\n}\n"), 0o644)
	writeExisting(t, jsonPath, []byte("{\n  \"mcp\": {}\n}\n"), 0o644)

	out := configureMCP(home, "opencode", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	body := string(readFile(t, jsonc))
	require.Contains(t, body, hostedMCPURL)
	require.Contains(t, body, "// keep")
	require.Equal(t, "{\n  \"mcp\": {}\n}\n", string(readFile(t, jsonPath)))
}

func TestOpenCodeJSONUsedWhenJSONCAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))

	out := configureMCP(home, "opencode", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.FileExists(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	_, err := os.Stat(filepath.Join(home, ".config", "opencode", "opencode.jsonc"))
	require.True(t, os.IsNotExist(err))
}

func TestConfigLocationsAreAbsoluteAndClean(t *testing.T) {
	base := canonicalTempDir(t)
	cwd := filepath.Join(base, "work")
	require.NoError(t, os.MkdirAll(cwd, 0o755))
	// Keep the working directory and config on the same Windows volume.
	t.Chdir(cwd)
	real := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(real, 0o755))
	rel, err := filepath.Rel(cwd, real)
	require.NoError(t, err)
	messy := filepath.Join(rel, "..", filepath.Base(real))
	require.Contains(t, filepath.ToSlash(messy), "..")

	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))

	t.Run("codex", func(t *testing.T) {
		t.Setenv("CODEX_HOME", messy)
		out := configureMCP(home, "codex", false, false)
		require.Equal(t, statusConfigured, out.Status, out.Detail)
		require.FileExists(t, filepath.Join(real, "config.toml"))
		require.NotContains(t, out.Detail, "..")
		require.Contains(t, out.Detail, real)
	})

	t.Run("claude", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", messy)
		out := configureMCP(home, "claude-code", false, false)
		require.Equal(t, statusConfigured, out.Status, out.Detail)
		require.FileExists(t, filepath.Join(real, ".claude.json"))
		require.NotContains(t, out.Detail, "..")
	})

	t.Run("copilot", func(t *testing.T) {
		t.Setenv("COPILOT_HOME", messy)
		out := configureMCP(home, "copilot", false, false)
		require.Equal(t, statusConfigured, out.Status, out.Detail)
		require.FileExists(t, filepath.Join(real, "mcp-config.json"))
		require.NotContains(t, out.Detail, "..")
	})
}

func TestXDGConfigHomeRelativeIsAbsolute(t *testing.T) {
	base := canonicalTempDir(t)
	cwd := filepath.Join(base, "work")
	require.NoError(t, os.MkdirAll(cwd, 0o755))
	// Keep the working directory and config on the same Windows volume.
	t.Chdir(cwd)
	real := filepath.Join(base, "xdg")
	require.NoError(t, os.MkdirAll(real, 0o755))
	rel, err := filepath.Rel(cwd, real)
	require.NoError(t, err)
	messy := filepath.Join(rel, "..", filepath.Base(real))
	require.Contains(t, filepath.ToSlash(messy), "..")

	home := t.TempDir()
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(home, ".fly"))
	t.Setenv("XDG_CONFIG_HOME", messy)

	out := configureMCP(home, "opencode", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	cfg := filepath.Join(real, "opencode", "opencode.json")
	require.FileExists(t, cfg)
	require.NotContains(t, out.Detail, "..")
	_, err = os.Stat(filepath.Join(home, ".config", "opencode", "opencode.json"))
	require.True(t, os.IsNotExist(err))
}

func TestOpenCodeDetectedFromXDGOnly(t *testing.T) {
	xdg := t.TempDir()
	ctx, home, flags, _, _, _ := harness(t)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	require.NoError(t, os.MkdirAll(filepath.Join(xdg, "opencode"), 0o755))
	require.NoError(t, flags.Set("yes", "true"))

	require.NoError(t, runAgent(ctx))
	require.Equal(t, hostedMCPURL, flyObject(t, filepath.Join(xdg, "opencode", "opencode.json"), "mcp")["url"])
	require.FileExists(t, filepath.Join(xdg, "opencode", "skills", "fly", "SKILL.md"))
	_, err := os.Stat(filepath.Join(home, ".config", "opencode"))
	require.True(t, os.IsNotExist(err))
}

func TestExplicitOpenCodeUsesXDGWhenDirMissing(t *testing.T) {
	xdg := t.TempDir()
	ctx, home, flags, _, _, _ := harness(t)
	dest := filepath.Join(xdg, "opencode")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("agent", "opencode"))

	require.NoError(t, runAgent(ctx))
	require.FileExists(t, filepath.Join(dest, "opencode.json"))
	require.FileExists(t, filepath.Join(dest, "skills", "fly", "SKILL.md"))
	_, err := os.Stat(filepath.Join(home, ".config", "opencode", "opencode.json"))
	require.True(t, os.IsNotExist(err))
}
