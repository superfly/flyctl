package helpers

import (
	"os"
	"path/filepath"
)

// GetConfigDirectory will return where config and state files should be
// stored, either respecting `FLY_CONFIG_DIR` or defaulting to the user's home
// directory at `~/.fly`.
func GetConfigDirectory() (string, error) {
	var dir string

	if value, isSet := os.LookupEnv("FLY_CONFIG_DIR"); isSet {
		dir = value
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(homeDir, ".fly")
	}

	return dir, nil
}
