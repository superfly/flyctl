package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag/flagctx"
)

// TestSendMetricsPrecedence tests that the env var overrides the config file if present
func TestSendMetricsPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		setEnv        bool
		configValue   bool
		expectedValue bool
	}{
		{
			name:          "env=1, config=true -> true",
			envValue:      "1",
			setEnv:        true,
			configValue:   true,
			expectedValue: true,
		},
		{
			name:          "env=1, config=false -> true",
			envValue:      "1",
			setEnv:        true,
			configValue:   false,
			expectedValue: true,
		},
		{
			name:          "env=0, config=true -> false",
			envValue:      "0",
			setEnv:        true,
			configValue:   true,
			expectedValue: false,
		},
		{
			name:          "env=0, config=false -> false",
			envValue:      "0",
			setEnv:        true,
			configValue:   false,
			expectedValue: false,
		},
		{
			name:          "env=unset, config=false -> false",
			envValue:      "1",
			setEnv:        false,
			configValue:   false,
			expectedValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, FileName)

			// Set environment variable if asked to
			if tt.setEnv {
				t.Setenv(SendMetricsEnvKey, tt.envValue)
			}

			// Create config file
			configContent := "send_metrics: " + boolToYAML(tt.configValue) + "\n"
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			require.NoError(t, err)

			// Load config
			ctx := flagctx.NewContext(context.Background(), pflag.NewFlagSet("test", pflag.ContinueOnError))
			cfg, err := Load(ctx, configPath)
			require.NoError(t, err)

			// Verify result
			assert.Equal(t, tt.expectedValue, cfg.SendMetrics,
				"Expected SendMetrics=%v with env=%s, env set=%v and config=%v",
				tt.expectedValue, tt.envValue, tt.setEnv, tt.configValue)
		})
	}
}

// TestSyntheticsAgentPrecedence tests that the env var overrides the config file if present
func TestSyntheticsAgentPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		setEnv        bool
		configValue   bool
		expectedValue bool
	}{
		{
			name:          "env=1, config=true -> true",
			envValue:      "1",
			setEnv:        true,
			configValue:   true,
			expectedValue: true,
		},
		{
			name:          "env=1, config=false -> true",
			envValue:      "1",
			setEnv:        true,
			configValue:   false,
			expectedValue: true,
		},
		{
			name:          "env=0, config=true -> false",
			envValue:      "0",
			setEnv:        true,
			configValue:   true,
			expectedValue: false,
		},
		{
			name:          "env=0, config=false -> false",
			envValue:      "0",
			setEnv:        true,
			configValue:   false,
			expectedValue: false,
		},
		{
			name:          "env=unset, config=false -> false",
			envValue:      "1",
			setEnv:        false,
			configValue:   false,
			expectedValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, FileName)

			// Set environment variable if asked to
			if tt.setEnv {
				t.Setenv(SyntheticsAgentEnvKey, tt.envValue)
			}

			// Create config file
			configContent := "synthetics_agent: " + boolToYAML(tt.configValue) + "\n"
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			require.NoError(t, err)

			// Load config
			ctx := flagctx.NewContext(context.Background(), pflag.NewFlagSet("test", pflag.ContinueOnError))
			cfg, err := Load(ctx, configPath)
			require.NoError(t, err)

			// Verify result
			assert.Equal(t, tt.expectedValue, cfg.SyntheticsAgent,
				"Expected SyntheticsAgent=%v with env=%s, env set=%v and config=%v",
				tt.expectedValue, tt.envValue, tt.setEnv, tt.configValue)
		})
	}
}

// Helper functions

func boolToYAML(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
