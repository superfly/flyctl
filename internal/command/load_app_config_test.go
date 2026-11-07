package command

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
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

func TestLoadAppConfigMachineConfigHint(t *testing.T) {
	for _, tc := range []struct {
		name, file, raw       string
		machineFlag, wantHint bool
	}{
		{"Machine restart object", "cfg.json", `{"image":"alpine:3.22","restart":{"policy":"on-failure","max_retries":3}}`, true, true},
		{"valid app restart array", "cfg.json", `{"app":"test-app","restart":[{"policy":"on-failure","retries":3}]}`, true, false},
		{"invalid restart scalar", "cfg.json", `{"restart":"invalid"}`, true, false},
		{"unrelated schema error", "cfg.json", `{"app":123}`, true, false},
		{"command without machine-config", "cfg.json", `{"restart":{"policy":"on-failure","max_retries":3}}`, false, false},
		{"TOML restart table", "fly.toml", "[restart]\npolicy = \"on-failure\"\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			require.NoError(t, os.WriteFile(path, []byte(tc.raw), 0o600))
			ctx := loadAppConfigTestContext(t, path)
			if tc.machineFlag {
				flag.FromContext(ctx).String("machine-config", "", "")
			}
			var output bytes.Buffer
			ctx = logger.NewContext(ctx, logger.New(&output, logger.Warn, false))

			loadedCtx, err := LoadAppConfigIfPresent(ctx)
			require.NoError(t, err)
			require.NotNil(t, appconfig.ConfigFromContext(loadedCtx))
			require.Equal(t, tc.wantHint, strings.Contains(output.String(), "--machine-config"), output.String())
			if tc.wantHint {
				require.Contains(t, output.String(), "json: cannot unmarshal object")
				var machine fly.MachineConfig
				require.NoError(t, config.ParseConfig(&machine, tc.raw))
				require.Equal(t, 3, machine.Restart.MaxRetries)
			}
			if tc.name == "valid app restart array" {
				cfg := appconfig.ConfigFromContext(loadedCtx)
				require.Empty(t, output.String())
				require.Len(t, cfg.Restart, 1)
				require.Equal(t, 3, cfg.Restart[0].MaxRetries)
			}
		})
	}
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
