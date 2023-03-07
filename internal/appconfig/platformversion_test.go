package appconfig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetMachinesPlatform(t *testing.T) {
	cfg := NewConfig()
	assert.NoError(t, cfg.SetMachinesPlatform())
	assert.NoError(t, cfg.SetPlatformVersion(MachinesPlatform))

	cfg.v2UnmarshalError = fmt.Errorf("Failed to parse fly.toml")
	assert.Error(t, cfg.SetMachinesPlatform())
	assert.Error(t, cfg.SetPlatformVersion(MachinesPlatform))
}

func TestSetNomadPlatform(t *testing.T) {
	cfg := NewConfig()
	assert.Error(t, cfg.SetNomadPlatform())
	assert.Error(t, cfg.SetPlatformVersion(NomadPlatform))

	cfg.RawDefinition["app"] = "foo"
	assert.NoError(t, cfg.SetNomadPlatform())
	assert.NoError(t, cfg.SetPlatformVersion(NomadPlatform))
}

func TestSetPlatformVersion(t *testing.T) {
	cfg := NewConfig()
	assert.Error(t, cfg.SetPlatformVersion(""))
	assert.Error(t, cfg.SetPlatformVersion("thenewkidsontheblock"))
}
