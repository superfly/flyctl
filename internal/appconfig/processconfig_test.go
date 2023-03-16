package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultProcessName_Nil(t *testing.T) {
	var cfg *Config
	assert.Equal(t, "app", cfg.DefaultProcessName())
}

func TestGetDefaultProcessName_Default(t *testing.T) {
	cfg, err := LoadConfig("./testdata/processes-none.toml")
	assert.NoError(t, err)
	// Test unknown platform version
	assert.Equal(t, "app", cfg.DefaultProcessName())
	// Test for machines
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "app", cfg.DefaultProcessName())
	// Test for nomad
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "app", cfg.DefaultProcessName())
}

func TestGetDefaultProcessName_First(t *testing.T) {
	cfg, err := LoadConfig("./testdata/processes-one.toml")
	assert.NoError(t, err)
	// Test unknown platform version
	assert.Equal(t, "web", cfg.DefaultProcessName())
	// Test for machines
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "web", cfg.DefaultProcessName())
	// Test for nomad
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "web", cfg.DefaultProcessName())
}

func TestGetDefaultProcessName_Many(t *testing.T) {
	cfg, err := LoadConfig("./testdata/processes-multi.toml")
	assert.NoError(t, err)
	// Test unknown platform version
	assert.Equal(t, "bar", cfg.DefaultProcessName())
	// Test for machines
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "bar", cfg.DefaultProcessName())
	// Test for nomad
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "bar", cfg.DefaultProcessName())
}

func TestGetDefaultProcessName_ManyWithApp(t *testing.T) {
	cfg, err := LoadConfig("./testdata/processes-multiwithapp.toml")
	assert.NoError(t, err)
	// Test unknown platform version
	assert.Equal(t, "app", cfg.DefaultProcessName())
	// Test for machines
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "app", cfg.DefaultProcessName())
	// Test for nomad
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.Equal(t, "app", cfg.DefaultProcessName())
}
