package command

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
)

func writeFlyToml(tb testing.TB, dir string) string {
	tb.Helper()
	path := filepath.Join(dir, appconfig.DefaultConfigFileName)
	content := []byte("app = \"myapp\"\n")
	require.NoError(tb, os.WriteFile(path, content, 0o644))
	return path
}

func newTestContext(wd string, fs *pflag.FlagSet) context.Context {
	ctx := context.Background()
	l := logger.New(&bytes.Buffer{}, logger.Debug, false)
	ctx = logger.NewContext(ctx, l)
	ctx = state.WithWorkingDirectory(ctx, wd)
	ctx = flag.NewContext(ctx, fs)
	return ctx
}

func TestLoadAppConfigIfPresent_ExplicitFile(t *testing.T) {
	tmp := t.TempDir()
	filePath := writeFlyToml(t, tmp)

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.StringP("config", "c", "", "")
	require.NoError(t, fs.Set("config", filePath))

	ctxIn := newTestContext(tmp, fs)
	ctxOut, err := LoadAppConfigIfPresent(ctxIn)
	require.NoError(t, err)

	cfg := appconfig.ConfigFromContext(ctxOut)
	require.NotNil(t, cfg)
	require.Equal(t, "myapp", cfg.AppName)
}

func TestLoadAppConfigIfPresent_NonExistentPath(t *testing.T) {
	tmp := t.TempDir()
	nonExist := filepath.Join(tmp, "does-not-exist.toml")

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.StringP("config", "c", "", "")
	require.NoError(t, fs.Set("config", nonExist))

	ctxIn := newTestContext(tmp, fs)
	_, err := LoadAppConfigIfPresent(ctxIn)
	require.Error(t, err)
}

func TestLoadAppConfigIfPresent_AutoDiscovery(t *testing.T) {
	tmp := t.TempDir()
	_ = writeFlyToml(t, tmp)

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	ctxIn := newTestContext(tmp, fs)
	ctxOut, err := LoadAppConfigIfPresent(ctxIn)
	require.NoError(t, err)
	cfg := appconfig.ConfigFromContext(ctxOut)
	require.NotNil(t, cfg)
}
