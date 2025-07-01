package containerconfig

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	fly "github.com/superfly/fly-go"
)

// ParseMachineConfig parses a machine config from JSON (either as a string or from a file).
func ParseMachineConfig(machineConfigStr string) (*fly.MachineConfig, error) {
	var mConfig fly.MachineConfig

	if machineConfigStr == "" {
		return nil, nil
	}

	var buf []byte
	// Check if it's a file path or inline JSON
	if _, err := os.Stat(machineConfigStr); err == nil {
		// It's a file path
		data, err := os.ReadFile(machineConfigStr)
		if err != nil {
			return nil, fmt.Errorf("failed to read machine config file: %w", err)
		}
		buf = data
	} else {
		// It's inline JSON
		buf = []byte(machineConfigStr)
	}

	if err := json.Unmarshal(buf, &mConfig); err != nil {
		return nil, fmt.Errorf("failed to parse machine config: %w", err)
	}

	// Process local files referenced in the config
	if err := readLocalFiles(&mConfig, buf); err != nil {
		return nil, err
	}

	return &mConfig, nil
}

// readLocalFiles reads local files from the machine config and inserts their content into the config.
func readLocalFiles(config *fly.MachineConfig, buf []byte) error {
	clean := true

	if config.Files != nil {
		for _, file := range config.Files {
			if file.RawValue == nil && file.SecretName == nil {
				clean = false
			}
		}
	}

	for _, container := range config.Containers {
		if container.Files != nil {
			for _, file := range container.Files {
				if file.RawValue == nil && file.SecretName == nil {
					clean = false
				}
			}
		}
	}

	if clean {
		return nil
	}

	// File represents a file configuration within a container
	type LocalFile struct {
		GuestPath string `json:"guest_path"`
		LocalPath string `json:"local_path"`
	}

	// Container represents a container configuration
	type LocalContainer struct {
		Name  string      `json:"name"`
		Files []LocalFile `json:"files"`
	}

	// Config represents the overall CLI configuration
	type LocalConfig struct {
		Files      []LocalFile      `json:"files"`
		Containers []LocalContainer `json:"containers"`
	}

	// Read the JSON file
	var localConf LocalConfig
	if err := json.Unmarshal(buf, &localConf); err != nil {
		return fmt.Errorf("invalid machine config %s: %w", string(buf), err)
	}

	if config.Files != nil {
		for _, file := range config.Files {
			if file.RawValue == nil && file.SecretName == nil {
				for _, localFile := range localConf.Files {
					if file.GuestPath == localFile.GuestPath {
						if localFile.LocalPath == "" {
							continue
						}

						content, err := os.ReadFile(localFile.LocalPath)
						if err != nil {
							return fmt.Errorf("failed to read file at %s: %w", localFile.LocalPath, err)
						}

						encodedContent := base64.StdEncoding.EncodeToString(content)
						file.RawValue = &encodedContent
					}
				}
			}
		}
	}

	for _, container := range config.Containers {
		if container.Files != nil {
			for _, file := range container.Files {
				if file.RawValue == nil && file.SecretName == nil {
					for _, localContainer := range localConf.Containers {
						if container.Name == localContainer.Name {
							for _, localFile := range localContainer.Files {
								if file.GuestPath == localFile.GuestPath {
									if localFile.LocalPath == "" {
										continue
									}

									content, err := os.ReadFile(localFile.LocalPath)
									if err != nil {
										return fmt.Errorf("failed to read file at %s: %w", localFile.LocalPath, err)
									}

									encodedContent := base64.StdEncoding.EncodeToString(content)
									file.RawValue = &encodedContent
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// ParseComposeFile parses a Docker Compose file and converts it to machine config.
func ParseComposeFile(composePath string) (*fly.MachineConfig, error) {
	if _, err := os.Stat(composePath); err != nil {
		return nil, fmt.Errorf("compose file not found: %w", err)
	}

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
		return ParseMachineConfig(machineConfigStr)
	}

	return nil, nil
}
