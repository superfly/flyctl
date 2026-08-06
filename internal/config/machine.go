package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/superfly/fly-go"
)

func ParseConfig(config *fly.MachineConfig, mc string) error {
	var buf []byte
	switch {
	case strings.HasPrefix(mc, "{"):
		buf = []byte(mc)
	case strings.HasSuffix(mc, ".json"):
		fo, err := os.Open(mc)
		if err != nil {
			return err
		}
		buf, err = io.ReadAll(fo)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid machine config source: %q", mc)
	}

	if err := json.Unmarshal(buf, config); err != nil {
		return fmt.Errorf("invalid machine config %q: %w", mc, err)
	}

	if err := readLocalFiles(config, buf); err != nil {
		return err
	}

	return nil
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
