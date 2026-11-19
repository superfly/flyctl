package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPathToSocketExplicitOverride(t *testing.T) {
	t.Setenv(SocketPathEnvKey, filepath.Join(t.TempDir(), "custom.sock"))

	if got, want := PathToSocket(), os.Getenv(SocketPathEnvKey); got != want {
		t.Errorf("PathToSocket() = %q, want %q", got, want)
	}

	if got, want := SocketPathOverride(), os.Getenv(SocketPathEnvKey); got != want {
		t.Errorf("SocketPathOverride() = %q, want %q", got, want)
	}
}

func TestPathToSocketIsolated(t *testing.T) {
	t.Setenv(IsolatedEnvKey, "1")
	t.Setenv(SocketPathEnvKey, "")
	os.Unsetenv(SocketPathEnvKey)

	t.Setenv(SocketDirEnvKey, "")
	os.Unsetenv(SocketDirEnvKey)
	t.Setenv("TMPDIR", t.TempDir())

	got := PathToSocket()

	// a private directory of our own under the temp dir, not the temp dir
	dir := filepath.Dir(got)
	if filepath.Dir(dir) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(dir), "fly-agent-") {
		t.Errorf("PathToSocket() = %q, want it in a fly-agent-* directory under %q", got, os.TempDir())
	}
	// Windows has no unix permission bits; os.Stat reports 0777 for any
	// writable directory there, so the mode can only be checked elsewhere.
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("socket directory %s: mode %v, err %v; want 0700", dir, info.Mode().Perm(), err)
		}
	}
	if env := os.Getenv(SocketDirEnvKey); env != dir {
		t.Errorf("%s = %q, want %q", SocketDirEnvKey, env, dir)
	}
	if remove := SocketDirToRemove(); remove != dir {
		t.Errorf("SocketDirToRemove() = %q, want %q", remove, dir)
	}

	// the generated path must be exported for subprocesses and stable
	if env := os.Getenv(SocketPathEnvKey); env != got {
		t.Errorf("%s = %q, want %q", SocketPathEnvKey, env, got)
	}
	if again := PathToSocket(); again != got {
		t.Errorf("PathToSocket() = %q on second call, want %q", again, got)
	}
	if override := SocketPathOverride(); override != got {
		t.Errorf("SocketPathOverride() = %q, want %q", override, got)
	}
}

func TestRemoveSocketDir(t *testing.T) {
	t.Setenv(IsolatedEnvKey, "")
	os.Unsetenv(IsolatedEnvKey)

	dir := t.TempDir()
	socket := filepath.Join(dir, "agent.sock")
	t.Setenv(SocketPathEnvKey, socket)
	t.Setenv(SocketDirEnvKey, dir)

	for _, f := range []string{socket, socket + ".lock", socket + ".start.lock"} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemoveSocketDir(dir); err != nil {
		t.Fatalf("RemoveSocketDir() = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory still exists (err %v)", err)
	}

	// a directory holding anything of someone else's is not ours to wipe
	dir = t.TempDir()
	socket = filepath.Join(dir, "agent.sock")
	t.Setenv(SocketPathEnvKey, socket)
	t.Setenv(SocketDirEnvKey, dir)
	if err := os.WriteFile(filepath.Join(dir, "precious"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RemoveSocketDir(dir); err == nil {
		t.Error("RemoveSocketDir() removed a directory with foreign files in it")
	}
	if _, err := os.Stat(filepath.Join(dir, "precious")); err != nil {
		t.Errorf("foreign file is gone: %v", err)
	}
}

func TestSocketDirToRemoveRefusesForeignDir(t *testing.T) {
	t.Setenv(IsolatedEnvKey, "")
	os.Unsetenv(IsolatedEnvKey)
	t.Setenv(SocketDirEnvKey, t.TempDir())
	t.Setenv(SocketPathEnvKey, filepath.Join(t.TempDir(), "elsewhere.sock"))

	if got := SocketDirToRemove(); got != "" {
		t.Errorf("SocketDirToRemove() = %q, want empty for a socket outside the directory", got)
	}
}

func TestPathToSocketDefault(t *testing.T) {
	t.Setenv(SocketPathEnvKey, "")
	os.Unsetenv(SocketPathEnvKey)
	t.Setenv(IsolatedEnvKey, "")
	os.Unsetenv(IsolatedEnvKey)
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())

	if got, want := PathToSocket(), filepath.Join(os.Getenv("FLY_CONFIG_DIR"), "fly-agent.sock"); got != want {
		t.Errorf("PathToSocket() = %q, want %q", got, want)
	}

	if got := SocketPathOverride(); got != "" {
		t.Errorf("SocketPathOverride() = %q, want empty", got)
	}
}
