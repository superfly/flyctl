package ssh

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

// issueContext is a context for openCredentialFiles with no terminal, as an
// agent or CI job runs it, and the given --overwrite.
func issueContext(t *testing.T, overwrite bool) context.Context {
	t.Helper()

	fs := pflag.NewFlagSet("issue", pflag.ContinueOnError)
	fs.Bool("dotssh", false, "")
	fs.Bool("overwrite", false, "")
	if overwrite {
		require.NoError(t, fs.Set("overwrite", "true"))
	}

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)

	return flag.NewContext(ctx, fs)
}

// openWithin runs openCredentialFiles, failing the test if it hasn't returned
// within a few seconds. Before the fix, a rejected --overwrite path looped
// forever.
func openWithin(t *testing.T, ctx context.Context, path string) (pf, cf *os.File, got string, out string, err error) {
	t.Helper()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		pf, cf, got, err = openCredentialFiles(ctx, &buf, path)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("openCredentialFiles did not return: it loops on a path it rejected")
	}

	return pf, cf, got, buf.String(), err
}

func TestOpenCredentialFilesRejectsNonFlyKeyWithoutLooping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("someone else's key"), 0o600))

	_, _, _, out, err := openWithin(t, issueContext(t, true), path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "isn't a fly.io ed25519 private key")
	assert.False(t, prompt.IsNonInteractive(err), "the error should say why the path was rejected, not just that prompting failed")
	assert.Equal(t, 1, bytes.Count([]byte(out), []byte("isn't a fly.io")), "the rejection should be reported once")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "someone else's key", string(data), "a non-fly.io key must not be overwritten")
}

func TestOpenCredentialFilesRejectsUnreadableFileWithoutLooping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows ignores unix permission bits, so the file stays readable")
	}
	if os.Getuid() == 0 {
		t.Skip("root can read any file")
	}

	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("fly.io"), 0o200))

	_, _, _, out, err := openWithin(t, issueContext(t, true), path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't read it")
	assert.Contains(t, err.Error(), path)
	assert.ErrorIs(t, err, fs.ErrPermission, "the underlying read error stays reachable")
	assert.Equal(t, 1, strings.Count(out, "can't read it"), "the rejection should be reported once")
}

func TestOpenCredentialFilesExistingFileWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("fly.io"), 0o600))

	_, _, _, _, err := openWithin(t, issueContext(t, false), path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't open private key file")
	assert.ErrorIs(t, err, os.ErrExist)
}

func TestOpenCredentialFilesOverwritesFlyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("old fly.io key"), 0o600))

	pf, cf, got, _, err := openWithin(t, issueContext(t, true), path)

	require.NoError(t, err)
	require.NotNil(t, pf)
	require.NotNil(t, cf)
	defer pf.Close()
	defer cf.Close()
	assert.Equal(t, path, got)
	assert.FileExists(t, path+"-cert.pub")
}

func TestOpenCredentialFilesNewPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")

	pf, cf, got, _, err := openWithin(t, issueContext(t, false), path)

	require.NoError(t, err)
	defer pf.Close()
	defer cf.Close()
	assert.Equal(t, path, got)
}

func TestOpenCredentialFilesNoPathNonInteractive(t *testing.T) {
	_, _, _, _, err := openWithin(t, issueContext(t, false), "")

	assert.True(t, prompt.IsNonInteractive(err))
}

// dotsshContext is issueContext with --dotssh set and HOME pointed at a temp
// dir holding an empty .ssh directory, which it returns.
func dotsshContext(t *testing.T, overwrite bool) (context.Context, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	require.NoError(t, os.Mkdir(sshDir, 0o700))

	ctx := issueContext(t, overwrite)
	require.NoError(t, flag.FromContext(ctx).Set("dotssh", "true"))

	return ctx, sshDir
}

func TestOpenCredentialFilesOverwriteTruncatesFlyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("an old, much longer fly.io key"), 0o600))
	require.NoError(t, os.WriteFile(path+"-cert.pub", []byte("an old, much longer cert"), 0o600))

	pf, cf, _, _, err := openWithin(t, issueContext(t, true), path)
	require.NoError(t, err)

	_, err = pf.WriteString("new key")
	require.NoError(t, err)
	_, err = cf.WriteString("new cert")
	require.NoError(t, err)
	require.NoError(t, pf.Close())
	require.NoError(t, cf.Close())

	key, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new key", string(key), "the old key is replaced, not partly overwritten")

	cert, err := os.ReadFile(path + "-cert.pub")
	require.NoError(t, err)
	assert.Equal(t, "new cert", string(cert))
}

func TestOpenCredentialFilesWithoutOverwriteLeavesNonFlyKeyAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("someone else's key"), 0o600))

	_, _, _, _, err := openWithin(t, issueContext(t, false), path)

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrExist)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "someone else's key", string(data))
	assert.NoFileExists(t, path+"-cert.pub", "no certificate is written next to a key that was refused")
}

func TestOpenCredentialFilesNewFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows ignores unix permission bits")
	}

	path := filepath.Join(t.TempDir(), "id_ed25519")

	pf, cf, _, _, err := openWithin(t, issueContext(t, false), path)
	require.NoError(t, err)
	defer pf.Close()
	defer cf.Close()

	for _, p := range []string{path, path + "-cert.pub"} {
		info, err := os.Stat(p)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "%s must only be readable by its owner", p)
	}
}

func TestOpenCredentialFilesDotSSH(t *testing.T) {
	ctx, sshDir := dotsshContext(t, false)

	pf, cf, got, _, err := openWithin(t, ctx, "id_fly")

	require.NoError(t, err)
	defer pf.Close()
	defer cf.Close()
	assert.Equal(t, filepath.Join(sshDir, "id_fly"), filepath.Clean(got), "--dotssh puts the key under ~/.ssh")
	assert.FileExists(t, filepath.Join(sshDir, "id_fly-cert.pub"))
}

func TestOpenCredentialFilesDotSSHRejectsNonFlyKeyWithoutLooping(t *testing.T) {
	ctx, sshDir := dotsshContext(t, true)
	path := filepath.Join(sshDir, "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("someone else's key"), 0o600))

	_, _, _, out, err := openWithin(t, ctx, "id_ed25519")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "isn't a fly.io ed25519 private key")
	assert.Contains(t, filepath.ToSlash(err.Error()), filepath.ToSlash(sshDir), "the error names the full path under ~/.ssh")
	assert.Equal(t, 1, strings.Count(out, "isn't a fly.io"))

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "someone else's key", string(data))
}

// Before, failing to open the certificate file ended the loop anyway: the key
// had already been emptied, and runSSHIssue reported success.
func TestOpenCredentialFilesCertFailureKeepsExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, []byte("my current fly.io key"), 0o600))
	require.NoError(t, os.Mkdir(path+"-cert.pub", 0o700)) // can't be opened as a file

	pf, cf, _, out, err := openWithin(t, issueContext(t, true), path)

	require.Error(t, err)
	assert.Nil(t, pf)
	assert.Nil(t, cf)
	assert.Contains(t, err.Error(), "can't open certificate file")
	assert.Contains(t, err.Error(), path+"-cert.pub")
	assert.Equal(t, 1, strings.Count(out, "Can't open certificate file"))
	assert.True(t, strings.HasSuffix(out, "\n"), "the message ends its line")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "my current fly.io key", string(data), "the key must not be emptied when the certificate can't be written")
}

func TestOpenCredentialFilesCertFailureRemovesNewKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path+"-cert.pub", []byte("someone's cert"), 0o600))

	_, _, _, _, err := openWithin(t, issueContext(t, false), path)

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrExist)
	assert.NoFileExists(t, path, "an empty key file created on the way shouldn't be left behind")

	data, readErr := os.ReadFile(path + "-cert.pub")
	require.NoError(t, readErr)
	assert.Equal(t, "someone's cert", string(data))
}
