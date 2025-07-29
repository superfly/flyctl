package appconfig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetMachinesPlatform(t *testing.T) {
	cfg := NewConfig()
	assert.NoError(t, cfg.SetMachinesPlatform())

	cfg.v2UnmarshalError = fmt.Errorf("Failed to parse fly.toml")
	assert.Error(t, cfg.SetMachinesPlatform())
}
