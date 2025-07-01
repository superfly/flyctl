package containerconfig

import (
	"path/filepath"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
)

// ParseComposeFile parses a Docker Compose file and converts it to machine config.
func ParseComposeFile(mConfig *fly.MachineConfig, composePath string) error {
	return ParseComposeFileWithPath(mConfig, composePath)
}

// ParseContainerConfig determines the type of container configuration and parses it directly into mConfig
func ParseContainerConfig(mConfig *fly.MachineConfig, composePath, machineConfigStr, configFilePath string) error {
	// Check if compose file is specified
	if composePath != "" {
		// Make path relative to fly.toml directory if not absolute
		if !filepath.IsAbs(composePath) {
			configDir := filepath.Dir(configFilePath)
			composePath = filepath.Join(configDir, composePath)
		}
		return ParseComposeFile(mConfig, composePath)
	}

	// Fall back to machine config if specified
	if machineConfigStr != "" {
		return config.ParseConfig(mConfig, machineConfigStr)
	}

	return nil
}
