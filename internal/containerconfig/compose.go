package containerconfig

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Healthcheck *ComposeHealthcheck    `yaml:"healthcheck"`
	Extra       map[string]interface{} `yaml:",inline"`
}

// ComposeDependency represents a service dependency with conditions
type ComposeDependency struct {
	Condition string `yaml:"condition"`
	Required  bool   `yaml:"required"`
	Restart   bool   `yaml:"restart"`
}

// ServiceDependencies represents parsed dependencies for a service
type ServiceDependencies struct {
	Dependencies map[string]ComposeDependency
}

// DependencyCondition constants
const (
	DependencyConditionStarted               = "service_started"
	DependencyConditionHealthy               = "service_healthy"
	DependencyConditionCompletedSuccessfully = "service_completed_successfully"
)

// ComposeHealthcheck represents a health check configuration
type ComposeHealthcheck struct {
	Test        interface{} `yaml:"test"`
	Interval    string      `yaml:"interval"`
	Timeout     string      `yaml:"timeout"`
	Retries     int         `yaml:"retries"`
	StartPeriod string      `yaml:"start_period"`
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

// parseDependsOn parses both short and long syntax depends_on
func parseDependsOn(dependsOn interface{}) (ServiceDependencies, error) {
	deps := ServiceDependencies{
		Dependencies: make(map[string]ComposeDependency),
	}

	if dependsOn == nil {
		return deps, nil
	}

	switch v := dependsOn.(type) {
	case []interface{}:
		// Short syntax: depends_on: [db, redis]
		for _, dep := range v {
			if serviceName, ok := dep.(string); ok {
				deps.Dependencies[serviceName] = ComposeDependency{
					Condition: DependencyConditionStarted,
					Required:  true,
					Restart:   false,
				}
			}
		}
	case map[string]interface{}:
		// Long syntax: depends_on: { db: { condition: service_healthy } }
		for serviceName, depConfig := range v {
			dependency := ComposeDependency{
				Condition: DependencyConditionStarted,
				Required:  true,
				Restart:   false,
			}

			if config, ok := depConfig.(map[string]interface{}); ok {
				if condition, exists := config["condition"]; exists {
					if condStr, ok := condition.(string); ok {
						dependency.Condition = condStr
					}
				}
				if required, exists := config["required"]; exists {
					if reqBool, ok := required.(bool); ok {
						dependency.Required = reqBool
					}
				}
				if restart, exists := config["restart"]; exists {
					if restartBool, ok := restart.(bool); ok {
						dependency.Restart = restartBool
					}
				}
			}

			deps.Dependencies[serviceName] = dependency
		}
	default:
		return deps, fmt.Errorf("invalid depends_on format")
	}

	return deps, nil
}

// parseVolume parses a Docker Compose volume string
// Format: [HOST:]CONTAINER[:ro|:rw]
func parseVolume(volume string) (hostPath, containerPath string, readOnly bool) {
	parts := strings.Split(volume, ":")

	switch len(parts) {
	case 1:
		// Just container path (anonymous volume)
		return "", parts[0], false
	case 2:
		// Could be HOST:CONTAINER or CONTAINER:ro
		if parts[1] == "ro" || parts[1] == "rw" {
			return "", parts[0], parts[1] == "ro"
		}
		return parts[0], parts[1], false
	case 3:
		// HOST:CONTAINER:ro/rw
		return parts[0], parts[1], parts[2] == "ro"
	default:
		// Invalid format, return container path from first part
		return "", parts[0], false
	}
}

// convertHealthcheck converts a compose healthcheck to Fly healthcheck
func convertHealthcheck(composeHC *ComposeHealthcheck) *fly.ContainerHealthcheck {
	if composeHC == nil {
		return nil
	}

	hc := &fly.ContainerHealthcheck{
		Name: "healthcheck", // Default name
	}

	// Parse test command
	var cmd []string
	switch test := composeHC.Test.(type) {
	case string:
		// HEALTHCHECK test
		cmd = []string{test}
	case []interface{}:
		// ["CMD", "wget", "--spider", "localhost:80"]
		for i, t := range test {
			if str, ok := t.(string); ok {
				// Skip "CMD" or "CMD-SHELL" prefix
				if i == 0 && (str == "CMD" || str == "CMD-SHELL") {
					continue
				}
				cmd = append(cmd, str)
			}
		}
	}

	// Set up exec healthcheck
	if len(cmd) > 0 {
		hc.ContainerHealthcheckType = fly.ContainerHealthcheckType{
			Exec: &fly.ExecHealthcheck{
				Command: cmd,
			},
		}
	}

	// Parse durations - for now just use defaults
	// In a real implementation, you'd parse "30s" -> 30, etc.
	if composeHC.Interval != "" {
		hc.Interval = 30 // Default 30s
	}
	if composeHC.Timeout != "" {
		hc.Timeout = 10 // Default 10s
	}
	if composeHC.Retries > 0 {
		hc.FailureThreshold = int32(composeHC.Retries)
	}

	return hc
}

// composeToMachineConfig converts a Docker Compose file to Fly machine configuration
// Always uses containers for compose files, regardless of service count
func composeToMachineConfig(mConfig *fly.MachineConfig, compose *ComposeFile, composePath string) error {
	if len(compose.Services) == 0 {
		return fmt.Errorf("no services defined in compose file")
	}

	// Initialize empty slices/maps if they don't exist
	if mConfig.Containers == nil {
		mConfig.Containers = []*fly.ContainerConfig{}
	}
	if mConfig.Restart == nil {
		mConfig.Restart = &fly.MachineRestart{}
	}

	// Parse dependencies for all services
	serviceDependencies := make(map[string]ServiceDependencies)
	for serviceName, service := range compose.Services {
		deps, err := parseDependsOn(service.DependsOn)
		if err != nil {
			return fmt.Errorf("failed to parse dependencies for service '%s': %w", serviceName, err)
		}
		serviceDependencies[serviceName] = deps
	}

	// Create containers for all services
	containers := make([]*fly.ContainerConfig, 0, len(compose.Services))

	// Check that only one service specifies build
	buildServiceCount := 0
	for _, service := range compose.Services {
		if service.Build != nil {
			buildServiceCount++
		}
	}
	if buildServiceCount > 1 {
		return fmt.Errorf("only one service can specify build, found %d services with build", buildServiceCount)
	}

	// Process all services as containers
	for serviceName, service := range compose.Services {
		container := &fly.ContainerConfig{
			Name: serviceName,
		}

		// Set image
		if service.Build != nil {
			// Service with build section uses "." as image
			container.Image = "."
		} else if service.Image != "" {
			container.Image = service.Image
		} else {
			// Services without build must specify image
			return fmt.Errorf("service '%s' must specify either 'image' or 'build'", serviceName)
		}

		// Handle environment variables
		if len(service.Environment) > 0 {
			container.ExtraEnv = make(map[string]string)
			for k, v := range service.Environment {
				container.ExtraEnv[k] = v
			}
		}

		// Handle compose-specific entrypoint/command if specified
		if service.Entrypoint != nil {
			switch ep := service.Entrypoint.(type) {
			case string:
				container.EntrypointOverride = []string{ep}
			case []interface{}:
				epSlice := make([]string, 0, len(ep))
				for _, e := range ep {
					if str, ok := e.(string); ok {
						epSlice = append(epSlice, str)
					}
				}
				container.EntrypointOverride = epSlice
			}
		}

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

		// If no entrypoint/command specified in compose, let container use image defaults

		// Handle user
		if service.User != "" {
			container.UserOverride = service.User
		}

		// Start with empty files list
		files := []*fly.File{}

		// Handle volume mounts
		for _, vol := range service.Volumes {
			hostPath, containerPath, readOnly := parseVolume(vol)
			if hostPath != "" {
				// Make host path absolute if relative
				if !filepath.IsAbs(hostPath) {
					hostPath = filepath.Join(filepath.Dir(composePath), hostPath)
				}

				// Read the file content
				content, err := os.ReadFile(hostPath)
				if err != nil {
					// Log warning but continue
					fmt.Printf("Warning: Could not read volume file %s: %v\n", hostPath, err)
					continue
				}

				// Add file to container
				encodedContent := base64.StdEncoding.EncodeToString(content)
				mode := uint32(0644)
				if readOnly {
					mode = 0444
				}

				files = append(files, &fly.File{
					GuestPath: containerPath,
					RawValue:  &encodedContent,
					Mode:      mode,
				})
			}
		}

		container.Files = files

		// Handle health checks
		if service.Healthcheck != nil {
			healthcheck := convertHealthcheck(service.Healthcheck)
			if healthcheck != nil {
				container.Healthchecks = []fly.ContainerHealthcheck{*healthcheck}
			}
		}

		// Handle dependencies
		if deps, exists := serviceDependencies[serviceName]; exists && len(deps.Dependencies) > 0 {
			var containerDeps []fly.ContainerDependency
			for depName, dep := range deps.Dependencies {
				var condition fly.ContainerDependencyCondition
				switch dep.Condition {
				case DependencyConditionStarted:
					condition = fly.Started
				case DependencyConditionHealthy:
					condition = fly.Healthy
				case DependencyConditionCompletedSuccessfully:
					condition = fly.ExitedSuccessfully
				default:
					condition = fly.Started // default fallback
				}

				containerDeps = append(containerDeps, fly.ContainerDependency{
					Name:      depName,
					Condition: condition,
				})
			}
			container.DependsOn = containerDeps
		}

		containers = append(containers, container)
	}

	mConfig.Containers = containers

	// Clear services - containers handle their own networking
	mConfig.Services = nil

	// Clear the main image - containers have their own images
	mConfig.Image = ""

	return nil
}

// ParseComposeFileWithPath parses a Docker Compose file and converts it to machine config
func ParseComposeFileWithPath(mConfig *fly.MachineConfig, composePath string) error {
	compose, err := parseComposeFile(composePath)
	if err != nil {
		return err
	}

	return composeToMachineConfig(mConfig, compose, composePath)
}
