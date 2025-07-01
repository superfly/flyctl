package containerconfig

import (
	"fmt"
	"os"

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

// composeToMachineConfig converts a Docker Compose file to Fly machine configuration
func composeToMachineConfig(compose *ComposeFile, composePath string) (*fly.MachineConfig, error) {
	if len(compose.Services) == 0 {
		return nil, fmt.Errorf("no services defined in compose file")
	}

	// For now, we'll support a subset of Docker Compose functionality
	// Starting with single-service compose files
	if len(compose.Services) > 1 {
		return nil, fmt.Errorf("multi-service compose files are not yet supported")
	}

	mConfig := &fly.MachineConfig{
		Init:    fly.MachineInit{},
		Restart: &fly.MachineRestart{},
	}

	// Get the first (and only) service
	for serviceName, service := range compose.Services {
		// Set the image
		if service.Image != "" {
			mConfig.Image = service.Image
		} else if service.Build != nil {
			// If build is specified, we'll need to handle this differently
			return nil, fmt.Errorf("compose files with build sections are not yet supported, please specify an image")
		}

		// Handle environment variables
		if len(service.Environment) > 0 {
			mConfig.Env = make(map[string]string)
			for k, v := range service.Environment {
				mConfig.Env[k] = v
			}
		}

		// Handle command
		if service.Command != nil {
			switch cmd := service.Command.(type) {
			case string:
				mConfig.Init.Cmd = []string{cmd}
			case []interface{}:
				cmdSlice := make([]string, 0, len(cmd))
				for _, c := range cmd {
					if str, ok := c.(string); ok {
						cmdSlice = append(cmdSlice, str)
					}
				}
				mConfig.Init.Cmd = cmdSlice
			}
		}

		// Handle entrypoint
		if service.Entrypoint != nil {
			switch ep := service.Entrypoint.(type) {
			case string:
				mConfig.Init.Entrypoint = []string{ep}
			case []interface{}:
				epSlice := make([]string, 0, len(ep))
				for _, e := range ep {
					if str, ok := e.(string); ok {
						epSlice = append(epSlice, str)
					}
				}
				mConfig.Init.Entrypoint = epSlice
			}
		}

		// Handle ports
		if len(service.Ports) > 0 {
			services := make([]fly.MachineService, 0)
			for range service.Ports {
				// Parse port specifications like "8080:80" or "80"
				// This is a simplified implementation
				// TODO: Handle more complex port specifications
				service := fly.MachineService{
					Protocol:     "tcp",
					InternalPort: 80, // Default, should be parsed from portSpec
				}
				services = append(services, service)
			}
			mConfig.Services = services
		}

		// Handle restart policy
		if service.Restart != "" {
			switch service.Restart {
			case "always":
				mConfig.Restart.Policy = fly.MachineRestartPolicyAlways
			case "on-failure":
				mConfig.Restart.Policy = fly.MachineRestartPolicyOnFailure
			case "no", "never":
				mConfig.Restart.Policy = fly.MachineRestartPolicyNo
			}
		}

		// Log which service we're using
		if len(compose.Services) == 1 {
			fmt.Printf("Using service '%s' from compose file\n", serviceName)
		}
	}

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
