package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConfigDirectoryWithEnv(t *testing.T) {
	var previousEnv string
	if v, isSet := os.LookupEnv("FLY_CONFIG_DIR"); isSet {
		previousEnv = v
	}

	os.Setenv("FLY_CONFIG_DIR", "/var/db/flyctl")

	value, err := GetConfigDirectory()
	assert.NoError(t, err)
	assert.Equal(t, "/var/db/flyctl", value)

	if previousEnv != "" {
		os.Setenv("FLY_CONFIG_DIR", previousEnv)
	}
}

func TestGetConfigDirectoryDefault(t *testing.T) {
	var previousEnv string
	if v, isSet := os.LookupEnv("FLY_CONFIG_DIR"); isSet {
		previousEnv = v
	}

	os.Unsetenv("FLY_CONFIG_DIR")

	value, err := GetConfigDirectory()
	assert.NoError(t, err)

	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)

	assert.Equal(t, filepath.Join(homeDir, ".fly"), value)

	if previousEnv != "" {
		os.Setenv("FLY_CONFIG_DIR", previousEnv)
	}
}
