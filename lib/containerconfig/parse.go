package containerconfig

import (
	"fmt"
	"path/filepath"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/lib/config"
)

// ParseContainerConfig determines the type of container configuration and parses it directly into mConfig
func ParseContainerConfig(mConfig *fly.MachineConfig, composePath, machineConfigStr, configFilePath, containerName string) error {
	var selectedContainer *fly.ContainerConfig

	// Check if compose file is specified
	if composePath != "" {
		// Make path relative to fly.toml directory if not absolute
		if !filepath.IsAbs(composePath) {
			configDir := filepath.Dir(configFilePath)
			composePath = filepath.Join(configDir, composePath)
		}
		if err := ParseComposeFileWithPath(mConfig, composePath); err != nil {
			return err
		}
	} else if machineConfigStr != "" {
		// Fall back to machine config if specified
		if err := config.ParseConfig(mConfig, machineConfigStr); err != nil {
			return err
		}

		// Apply container selection logic only for machine config JSON
		if len(mConfig.Containers) > 0 {
			// Select which container should receive the built image
			// Priority: specified containerName > "app" container > first container
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
		}
	} else {
		return nil
	}

	// Validate all containers have images and apply selectedContainer logic
	for _, c := range mConfig.Containers {
		if c == selectedContainer {
			// For machine config, set the selected container's image to "."
			c.Image = "."
		} else if c.Image == "" {
			// All other containers must have an image specified
			return fmt.Errorf("container %q must have an image specified", c.Name)
		}
	}

	return nil
}
