package ssh

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
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

	_, _, _, _, err := openWithin(t, issueContext(t, true), path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't read it")
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
