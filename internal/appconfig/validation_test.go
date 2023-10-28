package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateChecksSection(t *testing.T) {
	testCases := []struct {
		name           string
		configFilePath string
		expectError    bool
	}{
		{
			name:           "Test valid checks section",
			configFilePath: "testdata/validation-checks.toml",
			expectError:    false,
		},
		{
			name:           "Test invalid checks section",
			configFilePath: "testdata/validation-checks-invalid.toml",
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig(tc.configFilePath)
			assert.NoError(t, err)

			_, err = cfg.validateChecksSection()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateServicesSection(t *testing.T) {
	testCases := []struct {
		name           string
		configFilePath string
		expectError    bool
	}{
		{
			name:           "Test valid services section with 1 service",
			configFilePath: "testdata/validation-services-1.toml",
			expectError:    false,
		},
		{
			name:           "Test valid services section with 2 services",
			configFilePath: "testdata/validation-services-2.toml",
			expectError:    false,
		},
		{
			name:           "Test valid services section with 3 services",
			configFilePath: "testdata/validation-services-3.toml",
			expectError:    false,
		},
		{
			name:           "Test valid services section with 2 services and multi processes",
			configFilePath: "testdata/validation-services-2-multi-processes.toml",
			expectError:    false,
		},
		{
			name:           "Test valid services section with no services",
			configFilePath: "testdata/validation-services-0.toml",
			expectError:    false,
		},
		{
			name:           "Test invalid services section with 2 duplicate services",
			configFilePath: "testdata/validation-services-2-duplicate.toml",
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig(tc.configFilePath)
			assert.NoError(t, err)

			_, err = cfg.validateServicesSection()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateProcessesSection(t *testing.T) {
	testCases := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "Test valid processes section",
			config: &Config{
				Processes: map[string]string{
					"test": "bash",
				},
			},
			expectError: false,
		},
		{
			name: "Test invalid processes section",
			config: &Config{
				Processes: map[string]string{
					"test": "bash \"unclosed",
				},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.config.validateProcessesSection()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateConsoleCommand(t *testing.T) {
	testCases := []struct {
		name           string
		consoleCommand string
		expectError    bool
	}{
		{
			name:           "Test valid console command",
			consoleCommand: "bash",
			expectError:    false,
		},
		{
			name:           "Test invalid console command",
			consoleCommand: "bash \"unclosed",
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				ConsoleCommand: tc.consoleCommand,
			}

			_, err := cfg.validateConsoleCommand()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
