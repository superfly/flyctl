package containerconfig

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	fly "github.com/superfly/fly-go"
	"gopkg.in/yaml.v3"
)

// ComposeService represents a service definition in Docker Compose
type ComposeService struct {
	Image       string                 `yaml:"image"`
	Build       interface{}            `yaml:"build"`
	Environment map[string]string      `yaml:"environment"`
	Volumes     []string               `yaml:"volumes"`
	Ports       []string               `yaml:"ports"`
	Command     interface{}            `yaml:"command"`
	Entrypoint  interface{}            `yaml:"entrypoint"`
	WorkingDir  string                 `yaml:"working_dir"`
	User        string                 `yaml:"user"`
	Restart     string                 `yaml:"restart"`
	Configs     []interface{}          `yaml:"configs"`
	Secrets     []interface{}          `yaml:"secrets"`
	Deploy      map[string]interface{} `yaml:"deploy"`
	DependsOn   interface{}            `yaml:"depends_on"`
	Extra       map[string]interface{} `yaml:",inline"`
}

// ComposeFile represents a Docker Compose file structure
type ComposeFile struct {
	Version  string                    `yaml:"version"`
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]interface{}    `yaml:"volumes"`
	Networks map[string]interface{}    `yaml:"networks"`
	Configs  map[string]interface{}    `yaml:"configs"`
	Secrets  map[string]interface{}    `yaml:"secrets"`
}

// parseComposeFile reads and parses a Docker Compose YAML file
func parseComposeFile(composePath string) (*ComposeFile, error) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	return &compose, nil
}

// createHostsUpdateScript creates a shell script that updates /etc/hosts for service networking
func createHostsUpdateScript(serviceNames []string) string {
	script := `#!/bin/sh
# Update /etc/hosts to map service names to localhost for Docker Compose compatibility
`
	for _, serviceName := range serviceNames {
		script += fmt.Sprintf("echo '127.0.0.1 %s' >> /etc/hosts\n", serviceName)
	}

	script += `
# Execute the original entrypoint/command
exec "$@"
`
	return script
}

// extractImageEntrypoint extracts the entrypoint from a Docker image by fetching its config
func extractImageEntrypoint(imageName string) ([]string, error) {
	// Try to get the image config from the registry
	config, err := crane.Config(imageName)
	if err != nil {
		// If we can't fetch the config, return empty entrypoint
		// This might happen for private images or network issues
		return []string{}, nil
	}

	// Parse the config JSON
	var imageConfig struct {
		Config struct {
			Entrypoint []string `json:"Entrypoint"`
			Cmd        []string `json:"Cmd"`
		} `json:"config"`
	}

	if err := json.Unmarshal(config, &imageConfig); err != nil {
		return []string{}, nil
	}

	// Return the entrypoint if it exists
	if len(imageConfig.Config.Entrypoint) > 0 {
		return imageConfig.Config.Entrypoint, nil
	}

	// If no entrypoint, return empty (will use CMD or the image's default)
	return []string{}, nil
}

// composeToMachineConfig converts a Docker Compose file to Fly machine configuration
// Always uses containers for compose files, regardless of service count
func composeToMachineConfig(compose *ComposeFile, composePath string) (*fly.MachineConfig, error) {
	if len(compose.Services) == 0 {
		return nil, fmt.Errorf("no services defined in compose file")
	}

	mConfig := &fly.MachineConfig{
		Init:    fly.MachineInit{},
		Restart: &fly.MachineRestart{},
	}

	// Always use containers for compose files
	containers := make([]*fly.ContainerConfig, 0, len(compose.Services))

	// Find the "app" service to use as the main container, or use the first one
	var mainServiceName string
	var mainService *ComposeService

	// Check if there's an "app" service
	if appService, ok := compose.Services["app"]; ok {
		mainServiceName = "app"
		mainService = &appService
	} else {
		// Use the first service as main
		for name, svc := range compose.Services {
			mainServiceName = name
			mainService = &svc
			break
		}
	}

	// Set the main container image
	if mainService.Image != "" {
		mConfig.Image = mainService.Image
	} else if mainService.Build != nil {
		return nil, fmt.Errorf("compose files with build sections are not yet supported, please specify an image for service '%s'", mainServiceName)
	}

	// Collect all service names for hosts file
	serviceNames := make([]string, 0, len(compose.Services))
	for serviceName := range compose.Services {
		serviceNames = append(serviceNames, serviceName)
	}

	// Create the hosts update script
	hostsScript := createHostsUpdateScript(serviceNames)
	hostsScriptB64 := base64.StdEncoding.EncodeToString([]byte(hostsScript))

	// Process all services as containers
	for serviceName, service := range compose.Services {
		container := &fly.ContainerConfig{
			Name: serviceName,
		}

		// Set image
		if service.Image != "" {
			container.Image = service.Image
		} else if service.Build != nil {
			return nil, fmt.Errorf("compose files with build sections are not yet supported, please specify an image for service '%s'", serviceName)
		}

		// Handle environment variables
		if len(service.Environment) > 0 {
			container.ExtraEnv = make(map[string]string)
			for k, v := range service.Environment {
				container.ExtraEnv[k] = v
			}
		}

		// Handle command
		if service.Command != nil {
			switch cmd := service.Command.(type) {
			case string:
				container.CmdOverride = []string{cmd}
			case []interface{}:
				cmdSlice := make([]string, 0, len(cmd))
				for _, c := range cmd {
					if str, ok := c.(string); ok {
						cmdSlice = append(cmdSlice, str)
					}
				}
				container.CmdOverride = cmdSlice
			}
		}

		// Extract original entrypoint from image or service definition
		var originalEntrypoint []string
		if service.Entrypoint != nil {
			switch ep := service.Entrypoint.(type) {
			case string:
				originalEntrypoint = []string{ep}
			case []interface{}:
				epSlice := make([]string, 0, len(ep))
				for _, e := range ep {
					if str, ok := e.(string); ok {
						epSlice = append(epSlice, str)
					}
				}
				originalEntrypoint = epSlice
			}
		} else {
			// Try to extract from image
			originalEntrypoint, _ = extractImageEntrypoint(service.Image)
		}

		// Create wrapper entrypoint that updates hosts then runs original
		wrapperEntrypoint := []string{"/usr/local/bin/hosts-update.sh"}
		if len(originalEntrypoint) > 0 {
			wrapperEntrypoint = append(wrapperEntrypoint, originalEntrypoint...)
		}
		container.EntrypointOverride = wrapperEntrypoint

		// Handle user
		if service.User != "" {
			container.UserOverride = service.User
		}

		// Add the hosts update script as a file
		container.Files = []*fly.File{
			{
				GuestPath: "/usr/local/bin/hosts-update.sh",
				RawValue:  &hostsScriptB64,
				Mode:      0755, // Executable
			},
		}

		containers = append(containers, container)
	}

	mConfig.Containers = containers
	fmt.Printf("Using %d services from compose file as containers\n", len(compose.Services))
	fmt.Printf("Main container: '%s' (image: %s)\n", mainServiceName, mConfig.Image)

	return mConfig, nil
}

// ParseComposeFileWithPath parses a Docker Compose file and converts it to machine config
func ParseComposeFileWithPath(composePath string) (*fly.MachineConfig, error) {
	compose, err := parseComposeFile(composePath)
	if err != nil {
		return nil, err
	}

	return composeToMachineConfig(compose, composePath)
}
