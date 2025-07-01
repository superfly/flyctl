package containerconfig

import (
	"path/filepath"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
)

// ParseComposeFile parses a Docker Compose file and converts it to machine config.
func ParseComposeFile(composePath string) (*fly.MachineConfig, error) {
	return ParseComposeFileWithPath(composePath)
}

// ParseContainerConfig determines the type of container configuration and parses it accordingly
func ParseContainerConfig(composePath, machineConfigStr, configFilePath string) (*fly.MachineConfig, error) {
	// Check if compose file is specified
	if composePath != "" {
		// Make path relative to fly.toml directory if not absolute
		if !filepath.IsAbs(composePath) {
			configDir := filepath.Dir(configFilePath)
			composePath = filepath.Join(configDir, composePath)
		}
		return ParseComposeFile(composePath)
	}

	// Fall back to machine config if specified
	if machineConfigStr != "" {
		var mConfig fly.MachineConfig
		if err := config.ParseConfig(&mConfig, machineConfigStr); err != nil {
			return nil, err
		}
		return &mConfig, nil
	}

	return nil, nil
}