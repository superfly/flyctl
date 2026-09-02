package command

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
)

func TestLoadAppConfigIfPresent(t *testing.T) {
	t.Run("missing explicitly specified config returns an error", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "missing.toml")
		ctx := loadAppConfigTestContext(t, configPath)

		loadedCtx, err := LoadAppConfigIfPresent(ctx)
		if err == nil {
			t.Fatal("expected an error for a missing explicitly specified config")
		}
		if loadedCtx != nil {
			t.Fatal("expected a nil context when loading fails")
		}

		want := "config file not found at specified path: " + configPath + " (also tried: " + filepath.Join(configPath, appconfig.DefaultConfigFileName) + ")"
		if got := err.Error(); got != want {
			t.Fatalf("error = %q, want %q", got, want)
		}
	})

	t.Run("missing default config is allowed", func(t *testing.T) {
		ctx := loadAppConfigTestContext(t, "")

		loadedCtx, err := LoadAppConfigIfPresent(ctx)
		if err != nil {
			t.Fatalf("LoadAppConfigIfPresent() error = %v", err)
		}
		if loadedCtx != ctx {
			t.Fatal("expected the original context when no default config exists")
		}
	})

	t.Run("existing explicitly specified config is loaded", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "custom.toml")
		if err := os.WriteFile(configPath, []byte("app = \"test-app\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx := loadAppConfigTestContext(t, configPath)

		loadedCtx, err := LoadAppConfigIfPresent(ctx)
		if err != nil {
			t.Fatalf("LoadAppConfigIfPresent() error = %v", err)
		}
		if cfg := appconfig.ConfigFromContext(loadedCtx); cfg == nil {
			t.Fatal("expected the explicit config to be added to the context")
		}
	})
}

func loadAppConfigTestContext(t *testing.T, configPath string) context.Context {
	t.Helper()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String(flagnames.AppConfigFilePath, "", "")
	if configPath != "" {
		if err := fs.Set(flagnames.AppConfigFilePath, configPath); err != nil {
			t.Fatal(err)
		}
	}

	ctx := flag.NewContext(context.Background(), fs)
	ctx = logger.NewContext(ctx, logger.New(io.Discard, logger.NoLogLevel, false))

	return state.WithWorkingDirectory(ctx, t.TempDir())
}
