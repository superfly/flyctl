package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultProcessName_Nil(t *testing.T) {
	var cfg *Config
	assert.Equal(t, "app", cfg.GetDefaultProcessName())
}

func TestGetDefaultProcessName_Default(t *testing.T) {
	cfg, err := LoadConfig("./testdata/processes-none.toml")
	assert.NoError(t, err)
	// Test unknown platform version
	assert.Equal(t, "app", cfg.GetDefaultProcessName())
	// Test for machines
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "app", cfg.GetDefaultProcessName())
	// Test for nomad
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "app", cfg.GetDefaultProcessName())
}

func TestGetDefaultProcessName_First(t *testing.T) {
	cfg, err := LoadConfig("./testdata/processes-one.toml")
	assert.NoError(t, err)
	// Test unknown platform version
	assert.Equal(t, "web", cfg.GetDefaultProcessName())
	// Test for machines
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "web", cfg.GetDefaultProcessName())
	// Test for nomad
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "web", cfg.GetDefaultProcessName())
}
