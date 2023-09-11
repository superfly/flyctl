package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateServicesSection(t *testing.T) {
	cfg1, err := LoadConfig("testdata/validation-services-1.toml")
	assert.NoError(t, err)

	_, err = cfg1.validateServicesSection()
	assert.NoError(t, err)

	cfg2, err := LoadConfig("testdata/validation-services-2.toml")
	assert.NoError(t, err)

	_, err = cfg2.validateServicesSection()
	assert.NoError(t, err)

	cfg3, err := LoadConfig("testdata/validation-services-3.toml")
	assert.NoError(t, err)

	_, err = cfg3.validateServicesSection()
	assert.NoError(t, err)

	cfg2MultiProcesses, err := LoadConfig("testdata/validation-services-2-multi-processes.toml")
	assert.NoError(t, err)

	_, err = cfg2MultiProcesses.validateServicesSection()
	assert.NoError(t, err)

	cfg2Duplicate, err := LoadConfig("testdata/validation-services-2-duplicate.toml")
	assert.NoError(t, err)

	_, err = cfg2Duplicate.validateServicesSection()
	assert.Error(t, err)
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
