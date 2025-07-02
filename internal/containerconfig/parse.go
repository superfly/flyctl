package containerconfig

import (
	"fmt"
	"path/filepath"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
)

// ParseComposeFile parses a Docker Compose file and converts it to machine config.
func ParseComposeFile(mConfig *fly.MachineConfig, composePath string) error {
	return ParseComposeFileWithPath(mConfig, composePath)
}

// ParseContainerConfig determines the type of container configuration and parses it directly into mConfig
func ParseContainerConfig(mConfig *fly.MachineConfig, composePath, machineConfigStr, configFilePath, containerName string) error {
	// Check if compose file is specified
	if composePath != "" {
		// Make path relative to fly.toml directory if not absolute
		if !filepath.IsAbs(composePath) {
			configDir := filepath.Dir(configFilePath)
			composePath = filepath.Join(configDir, composePath)
		}
		if err := ParseComposeFile(mConfig, composePath); err != nil {
			return err
		}
	} else if machineConfigStr != "" {
		// Fall back to machine config if specified
		if err := config.ParseConfig(mConfig, machineConfigStr); err != nil {
			return err
		}
	} else {
		return nil
	}

	// After parsing, if we have containers, apply the updateContainerImage logic
	if len(mConfig.Containers) > 0 {
		// Copy the logic from updateContainerImage that selects the container
		var selectedContainer *fly.ContainerConfig

		match := containerName
		if match == "" {
			match = "app"
		}

		for _, c := range mConfig.Containers {
			if c.Name == match {
				selectedContainer = c
				break
			}
		}

		if selectedContainer == nil {
			if containerName != "" {
				return fmt.Errorf("container %q not found", containerName)
			} else {
				selectedContainer = mConfig.Containers[0]
			}
		}

		// Ensure that image is set in every container but the selected one
		// In the selected container, set image to "."
		for _, c := range mConfig.Containers {
			if c == selectedContainer {
				c.Image = "."
			} else if c.Image == "" {
				return fmt.Errorf("container %q must have an image specified", c.Name)
			}
		}
	}

	return nil
}
