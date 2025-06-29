package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/internal/command/launch/plan"
	"gopkg.in/yaml.v3"
)

// DockerComposeConfig represents a parsed docker-compose.yml file
type DockerComposeConfig struct {
	Version  string                          `yaml:"version"`
	Services map[string]DockerComposeService `yaml:"services"`
	Volumes  map[string]DockerComposeVolume  `yaml:"volumes,omitempty"`
	Networks map[string]interface{}          `yaml:"networks,omitempty"`
	Secrets  map[string]DockerComposeSecret  `yaml:"secrets,omitempty"`
}

// DockerComposeService represents a service in docker-compose.yml
type DockerComposeService struct {
	Image       string                    `yaml:"image,omitempty"`
	Build       interface{}               `yaml:"build,omitempty"` // can be string or BuildConfig
	Ports       []string                  `yaml:"ports,omitempty"`
	Environment interface{}               `yaml:"environment,omitempty"` // can be map or array
	Volumes     []string                  `yaml:"volumes,omitempty"`
	DependsOn   interface{}               `yaml:"depends_on,omitempty"` // can be array or map
	Command     interface{}               `yaml:"command,omitempty"`    // can be string or array
	Entrypoint  interface{}               `yaml:"entrypoint,omitempty"` // can be string or array
	WorkingDir  string                    `yaml:"working_dir,omitempty"`
	Restart     string                    `yaml:"restart,omitempty"`
	HealthCheck *DockerComposeHealthCheck `yaml:"healthcheck,omitempty"`
	Deploy      *DockerComposeDeploy      `yaml:"deploy,omitempty"`
	Networks    interface{}               `yaml:"networks,omitempty"`
	Labels      map[string]string         `yaml:"labels,omitempty"`
	ExtraHosts  []string                  `yaml:"extra_hosts,omitempty"`
	Privileged  bool                      `yaml:"privileged,omitempty"`
	ReadOnly    bool                      `yaml:"read_only,omitempty"`
	StdinOpen   bool                      `yaml:"stdin_open,omitempty"`
	Tty         bool                      `yaml:"tty,omitempty"`
	User        string                    `yaml:"user,omitempty"`
	Expose      []string                  `yaml:"expose,omitempty"`
	Secrets     interface{}               `yaml:"secrets,omitempty"` // can be array of strings or array of maps
}

// DockerComposeHealthCheck represents health check configuration
type DockerComposeHealthCheck struct {
	Test        interface{} `yaml:"test"` // can be string or array
	Interval    string      `yaml:"interval,omitempty"`
	Timeout     string      `yaml:"timeout,omitempty"`
	Retries     int         `yaml:"retries,omitempty"`
	StartPeriod string      `yaml:"start_period,omitempty"`
}

// DockerComposeDeploy represents deployment configuration
type DockerComposeDeploy struct {
	Replicas      int                         `yaml:"replicas,omitempty"`
	Resources     *DockerComposeResources     `yaml:"resources,omitempty"`
	RestartPolicy *DockerComposeRestartPolicy `yaml:"restart_policy,omitempty"`
}

// DockerComposeResources represents resource constraints
type DockerComposeResources struct {
	Limits       *DockerComposeResourceSpec `yaml:"limits,omitempty"`
	Reservations *DockerComposeResourceSpec `yaml:"reservations,omitempty"`
}

// DockerComposeResourceSpec represents CPU/memory specifications
type DockerComposeResourceSpec struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

// DockerComposeRestartPolicy represents restart policy configuration
type DockerComposeRestartPolicy struct {
	Condition   string `yaml:"condition,omitempty"`
	Delay       string `yaml:"delay,omitempty"`
	MaxAttempts int    `yaml:"max_attempts,omitempty"`
	Window      string `yaml:"window,omitempty"`
}

// DockerComposeVolume represents a volume definition
type DockerComposeVolume struct {
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
	External   bool              `yaml:"external,omitempty"`
	Name       string            `yaml:"name,omitempty"`
}

// DockerComposeBuildConfig represents build configuration
type DockerComposeBuildConfig struct {
	Context    string            `yaml:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
	Target     string            `yaml:"target,omitempty"`
}

// DockerComposeSecret represents a secret definition
type DockerComposeSecret struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

// configureDockerCompose detects and configures Docker Compose projects
func configureDockerCompose(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// Check for docker-compose files
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	var composeFile string
	for _, file := range composeFiles {
		path := filepath.Join(sourceDir, file)
		if absFileExists(path) {
			composeFile = path
			break
		}
	}

	if composeFile == "" {
		return nil, nil
	}

	// Parse the docker-compose file
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read docker-compose file: %w", err)
	}

	var compose DockerComposeConfig
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose file: %w", err)
	}

	if len(compose.Services) == 0 {
		return nil, fmt.Errorf("no services found in docker-compose file")
	}

	s := &SourceInfo{
		Family:     "Docker Compose",
		Version:    compose.Version,
		Notice:     "This application uses Docker Compose with multiple services",
		Containers: []Container{}, // Will be populated with service containers
		Secrets:    []Secret{},    // Will be populated with database credentials
		// Explicitly clear all build-related fields from any previous scanners
		Builder:        "",
		DockerfilePath: "",
		Buildpacks:     nil,
		BuildArgs:      nil,
	}

	// First pass: identify database services and propose plan
	for name, service := range compose.Services {
		// Detect database services and propose them in the plan
		if isPostgresService(name, service) {
			s.DatabaseDesired = DatabaseKindPostgres
		} else if isMySQLService(name, service) {
			s.DatabaseDesired = DatabaseKindMySQL
		} else if isRedisService(name, service) {
			s.RedisDesired = true
		}
	}

	// Second pass: check for build sections and validate they are identical
	buildServices := []string{}
	var firstBuildConfig *buildConfig

	for name, service := range compose.Services {
		// Check if service has a build section
		if service.Build != nil {
			// Extract build configuration
			var config buildConfig
			switch build := service.Build.(type) {
			case string:
				config.context = build
			case map[string]interface{}:
				if context, ok := build["context"].(string); ok {
					config.context = context
				}
				if dockerfile, ok := build["dockerfile"].(string); ok {
					config.dockerfile = dockerfile
				}
				if args, ok := build["args"].(map[string]interface{}); ok {
					config.args = make(map[string]string)
					for k, v := range args {
						config.args[k] = fmt.Sprintf("%v", v)
					}
				}
				if target, ok := build["target"].(string); ok {
					config.target = target
				}
			}

			// If this is the first build config, save it for comparison
			if firstBuildConfig == nil {
				firstBuildConfig = &config
			} else {
				// Validate that build configs are identical
				if !areBuildConfigsIdentical(firstBuildConfig, &config) {
					return nil, fmt.Errorf("multiple services with different build configurations found. All services with build sections must have identical build configurations")
				}
			}

			buildServices = append(buildServices, name)
		}
	}

	// If services have build sections, set up the build configuration
	if len(buildServices) > 0 {
		// The first service with a build section becomes the primary container
		s.Container = buildServices[0]
		// Track all services that should receive the built image
		s.BuildContainers = buildServices
	} else {
		// When all containers use external images, ensure no build configuration is set
		// This prevents the creation of an unnecessary [build] section in fly.toml
		s.Builder = ""
		s.DockerfilePath = ""
		s.Container = ""
		s.Buildpacks = nil
		s.BuildArgs = nil
		s.BuildContainers = nil
	}

	// Third pass: process ALL services initially (decisions about exclusion happen in callback)
	primaryService := ""
	for name, service := range compose.Services {

		// Extract port from the first non-database service
		if primaryService == "" && s.Port == 0 {
			port := extractServicePort(service)
			if port > 0 {
				s.Port = port
				primaryService = name
			}
		}

		// Prepare container configuration (no exclusions during scanning)
		container := prepareContainerFromService(name, service, sourceDir, len(compose.Services))

		// Check if DATABASE_URL or REDIS_URL exist before extraction
		hadDatabaseURL := false
		hadRedisURL := false
		if _, ok := container.Env["DATABASE_URL"]; ok {
			hadDatabaseURL = true
		}
		if _, ok := container.Env["REDIS_URL"]; ok {
			hadRedisURL = true
		}

		// Extract database credentials from environment variables
		// Only extract secrets for databases that are NOT being proposed as managed services
		secrets := extractDatabaseSecrets(container.Env, s.DatabaseDesired, s.RedisDesired)
		s.Secrets = append(s.Secrets, secrets...)

		// Add the secret names to the container's secrets list
		for _, secret := range secrets {
			container.Secrets = append(container.Secrets, secret.Key)
		}

		// Always add DATABASE_URL to container secrets if it was in the environment
		// This ensures container can access it whether it comes from managed DB or extracted secret
		if hadDatabaseURL {
			hasDBURL := false
			for _, secretName := range container.Secrets {
				if secretName == "DATABASE_URL" {
					hasDBURL = true
					break
				}
			}
			if !hasDBURL {
				container.Secrets = append(container.Secrets, "DATABASE_URL")
			}
		}

		// Similarly for REDIS_URL
		if hadRedisURL {
			hasRedisURL := false
			for _, secretName := range container.Secrets {
				if secretName == "REDIS_URL" {
					hasRedisURL = true
					break
				}
			}
			if !hasRedisURL {
				container.Secrets = append(container.Secrets, "REDIS_URL")
			}
		}

		s.Containers = append(s.Containers, container)
	}

	// If no port was found, use default
	if s.Port == 0 {
		s.Port = 8080
	}

	// Notices about databases will be handled in the callback based on final plan

	// Extract volumes
	for name, vol := range compose.Volumes {
		if !vol.External {
			s.Volumes = append(s.Volumes, Volume{
				Source:      name,
				Destination: fmt.Sprintf("/data/%s", name),
			})
		}
	}

	// Check if any containers need the service discovery entrypoint
	needsServiceDiscovery := false
	for _, container := range s.Containers {
		if len(container.Entrypoint) > 0 && container.Entrypoint[0] == "/fly-entrypoint.sh" {
			needsServiceDiscovery = true
			break
		}
	}

	// Add entrypoint script for service discovery only if needed
	if needsServiceDiscovery {
		s.Files = append(s.Files, SourceFile{
			Path:     "/fly-entrypoint.sh",
			Contents: generateEntrypointScript(s.Containers),
		})
	}

	// Process Docker Compose secrets
	if len(compose.Secrets) > 0 {
		composeSecrets := processDockerComposeSecrets(compose.Secrets, compose.Services, sourceDir)
		s.Secrets = append(s.Secrets, composeSecrets...)

		// Add secrets to containers that reference them
		for i := range s.Containers {
			container := &s.Containers[i]
			// Find the original service to check its secrets
			for serviceName, service := range compose.Services {
				if serviceName == container.Name {
					serviceSecrets := extractServiceSecrets(service.Secrets)
					container.Secrets = append(container.Secrets, serviceSecrets...)
					break
				}
			}
		}
	}

	// Add callback for additional configuration
	s.Callback = composeCallback

	return s, nil
}

// Helper functions to detect database services
func isPostgresService(name string, service DockerComposeService) bool {
	if strings.Contains(strings.ToLower(name), "postgres") ||
		strings.Contains(strings.ToLower(name), "postgresql") {
		return true
	}
	if strings.Contains(service.Image, "postgres") {
		return true
	}
	return false
}

func isMySQLService(name string, service DockerComposeService) bool {
	if strings.Contains(strings.ToLower(name), "mysql") ||
		strings.Contains(strings.ToLower(name), "mariadb") {
		return true
	}
	if strings.Contains(service.Image, "mysql") ||
		strings.Contains(service.Image, "mariadb") {
		return true
	}
	return false
}

func isRedisService(name string, service DockerComposeService) bool {
	if strings.Contains(strings.ToLower(name), "redis") {
		return true
	}
	if strings.Contains(service.Image, "redis") {
		return true
	}
	return false
}

// Helper functions for container identification in callback
func isPostgresContainer(name, image string) bool {
	return isPostgresService(name, DockerComposeService{Image: image})
}

func isRedisContainer(name, image string) bool {
	return isRedisService(name, DockerComposeService{Image: image})
}

// extractServicePort extracts the internal port from service configuration
func extractServicePort(service DockerComposeService) int {
	for _, portMapping := range service.Ports {
		// Port mappings can be:
		// - "8080"
		// - "8080:80"
		// - "127.0.0.1:8080:80"
		parts := strings.Split(portMapping, ":")
		var internalPort string
		if len(parts) == 1 {
			internalPort = parts[0]
		} else if len(parts) == 2 {
			internalPort = parts[1]
		} else if len(parts) == 3 {
			internalPort = parts[2]
		}

		// Remove any protocol suffix (e.g., "80/tcp")
		internalPort = strings.Split(internalPort, "/")[0]

		if port, err := strconv.Atoi(internalPort); err == nil {
			return port
		}
	}

	// Check expose ports
	for _, exposePort := range service.Expose {
		exposePort = strings.Split(exposePort, "/")[0]
		if port, err := strconv.Atoi(exposePort); err == nil {
			return port
		}
	}

	return 0
}

// prepareContainerFromService creates a Container configuration from a Docker Compose service
func prepareContainerFromService(name string, service DockerComposeService, sourceDir string, totalServices int) Container {
	container := Container{
		Name: name,
	}

	// Set image or build context
	if service.Image != "" {
		container.Image = service.Image
	} else if service.Build != nil {
		// Handle build configuration
		switch build := service.Build.(type) {
		case string:
			container.BuildContext = filepath.Join(sourceDir, build)
		case map[string]interface{}:
			if context, ok := build["context"].(string); ok {
				container.BuildContext = filepath.Join(sourceDir, context)
			}
			if dockerfile, ok := build["dockerfile"].(string); ok {
				container.Dockerfile = dockerfile
			}
		}
	}

	// Extract environment variables
	container.Env = make(map[string]string)
	switch env := service.Environment.(type) {
	case map[string]interface{}:
		for k, v := range env {
			container.Env[k] = fmt.Sprintf("%v", v)
		}
	case []interface{}:
		for _, e := range env {
			if str, ok := e.(string); ok {
				parts := strings.SplitN(str, "=", 2)
				if len(parts) == 2 {
					container.Env[parts[0]] = parts[1]
				}
			}
		}
	}

	// Set up entrypoint and command
	originalEntrypoint := extractCommand(service.Entrypoint)
	originalCommand := extractCommand(service.Command)

	// Determine whether to use image defaults
	hasExplicitEntrypoint := len(originalEntrypoint) > 0
	hasExplicitCommand := len(originalCommand) > 0

	if !hasExplicitEntrypoint && !hasExplicitCommand {
		// No entrypoint or command specified - use image defaults without any override
		// This is critical: we don't set ANY entrypoint or command
		container.Entrypoint = nil
		container.Command = nil
		container.UseImageDefaults = true
	} else {
		// Service has explicit entrypoint/command
		// For now, assume we might need service discovery (final decision in callback)
		// Use our entrypoint script for service discovery
		container.Entrypoint = []string{"/fly-entrypoint.sh"}

		// Chain to original entrypoint/command
		if hasExplicitEntrypoint {
			container.Command = append(originalEntrypoint, originalCommand...)
		} else {
			container.Command = originalCommand
		}
	}

	// Handle dependencies (will be filtered in callback based on final plan)
	container.DependsOn = extractDependencies(service.DependsOn, nil)

	// Handle health check
	if service.HealthCheck != nil {
		container.HealthCheck = convertHealthCheck(service.HealthCheck)
	}

	// Handle restart policy
	if service.Restart != "" {
		container.RestartPolicy = mapRestartPolicy(service.Restart)
	}

	// Process bind mount volumes
	for _, volume := range service.Volumes {
		if bindMount := parseBindMount(volume, sourceDir); bindMount != nil {
			container.Files = append(container.Files, *bindMount)
		}
	}

	return container
}

// extractCommand converts command/entrypoint to string array
func extractCommand(cmd interface{}) []string {
	if cmd == nil {
		return nil
	}

	switch c := cmd.(type) {
	case string:
		// Shell form - split by spaces (simple implementation)
		return strings.Fields(c)
	case []interface{}:
		// Exec form
		result := make([]string, len(c))
		for i, v := range c {
			result[i] = fmt.Sprintf("%v", v)
		}
		return result
	}
	return nil
}

// extractDependencies extracts service dependencies, filtering out excluded services
func extractDependencies(deps interface{}, excludedServices map[string]bool) []ContainerDependency {
	if deps == nil {
		return nil
	}

	var result []ContainerDependency

	switch d := deps.(type) {
	case []interface{}:
		// Simple array format
		for _, dep := range d {
			if name, ok := dep.(string); ok {
				// Skip dependencies on excluded services (if excludedServices provided)
				if excludedServices != nil && excludedServices[name] {
					continue
				}
				result = append(result, ContainerDependency{
					Name:      name,
					Condition: "started",
				})
			}
		}
	case map[string]interface{}:
		// Extended format with conditions
		for name, config := range d {
			// Skip dependencies on excluded services (if excludedServices provided)
			if excludedServices != nil && excludedServices[name] {
				continue
			}

			dependency := ContainerDependency{
				Name:      name,
				Condition: "started", // default
			}
			if cfg, ok := config.(map[string]interface{}); ok {
				if cond, ok := cfg["condition"].(string); ok {
					// Map docker-compose conditions to Fly.io conditions
					switch cond {
					case "service_healthy":
						dependency.Condition = "healthy"
					case "service_started":
						dependency.Condition = "started"
					case "service_completed_successfully":
						dependency.Condition = "exited_successfully"
					}
				}
			}
			result = append(result, dependency)
		}
	}

	return result
}

// convertHealthCheck converts Docker Compose health check to Fly.io format
func convertHealthCheck(hc *DockerComposeHealthCheck) *ContainerHealthCheck {
	if hc == nil {
		return nil
	}

	result := &ContainerHealthCheck{}

	// Convert test command
	switch test := hc.Test.(type) {
	case string:
		if test == "NONE" {
			return nil
		}
		result.Test = strings.Fields(test)
	case []interface{}:
		result.Test = make([]string, len(test))
		for i, v := range test {
			result.Test[i] = fmt.Sprintf("%v", v)
		}
	}

	// Convert durations
	result.Interval = hc.Interval
	result.Timeout = hc.Timeout
	result.StartPeriod = hc.StartPeriod
	result.Retries = hc.Retries

	return result
}

// composeCallback is called during the launch process after user confirms the plan
func composeCallback(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan, flags []string) error {
	// Now that we have the final plan, filter containers and secrets based on user decisions

	// Determine which services should be excluded based on the plan
	excludedServices := make(map[string]bool)

	// If user chose managed databases, exclude database containers
	if plan.Postgres.Provider() != nil {
		// Find PostgreSQL services and exclude them
		for i := len(srcInfo.Containers) - 1; i >= 0; i-- {
			container := srcInfo.Containers[i]
			if isPostgresContainer(container.Name, container.Image) {
				excludedServices[container.Name] = true
				// Remove from containers list
				srcInfo.Containers = append(srcInfo.Containers[:i], srcInfo.Containers[i+1:]...)
			}
		}
	}

	if plan.Redis.Provider() != nil {
		// Find Redis services and exclude them
		for i := len(srcInfo.Containers) - 1; i >= 0; i-- {
			container := srcInfo.Containers[i]
			if isRedisContainer(container.Name, container.Image) {
				excludedServices[container.Name] = true
				// Remove from containers list
				srcInfo.Containers = append(srcInfo.Containers[:i], srcInfo.Containers[i+1:]...)
			}
		}
	}

	// Filter secrets based on final plan
	// Remove secrets that Fly.io will automatically provide when creating managed databases
	filteredSecrets := []Secret{}
	for _, secret := range srcInfo.Secrets {
		// Skip DATABASE_URL if user chose managed Postgres (Fly launch will create this secret)
		if secret.Key == "DATABASE_URL" && plan.Postgres.Provider() != nil {
			continue // Fly.io will automatically create and set this secret
		}
		// Skip REDIS_URL if user chose managed Redis (Fly launch will create this secret)
		if secret.Key == "REDIS_URL" && plan.Redis.Provider() != nil {
			continue // Fly.io will automatically create and set this secret
		}
		filteredSecrets = append(filteredSecrets, secret)
	}
	srcInfo.Secrets = filteredSecrets

	// Update container dependencies to remove excluded services
	for i := range srcInfo.Containers {
		container := &srcInfo.Containers[i]
		filteredDeps := []ContainerDependency{}
		for _, dep := range container.DependsOn {
			if !excludedServices[dep.Name] {
				filteredDeps = append(filteredDeps, dep)
			}
		}
		container.DependsOn = filteredDeps
	}

	// Container secrets for managed databases will be available automatically
	// when the databases are attached (no action needed here)

	// Add any Docker Compose specific configuration or warnings
	if len(srcInfo.Containers) > 1 {
		fmt.Printf("\nConfiguring multi-container application with %d services\n", len(srcInfo.Containers))
		fmt.Println("Note: All containers will run in the same VM with shared networking.")
		fmt.Println("Containers can communicate with each other using localhost (127.0.0.1) and their respective port numbers.")
	} else if len(srcInfo.Containers) == 1 {
		// If only one container remains after database services are replaced,
		// clear multi-container configuration to use single-container deployment
		srcInfo.Containers = nil
		srcInfo.Container = ""
		srcInfo.BuildContainers = nil
	}

	return nil
}

// extractDatabaseSecrets identifies database-related environment variables and returns them as secrets
// It also removes them from the environment map
// Skip secrets that will be provided by managed services (when databases are proposed)
func extractDatabaseSecrets(env map[string]string, databaseDesired DatabaseKind, redisDesired bool) []Secret {
	var secrets []Secret

	// Common database-related environment variable patterns
	databasePatterns := []string{
		"DATABASE_URL",
		"DB_URL",
		"POSTGRES_URL",
		"POSTGRESQL_URL",
		"MYSQL_URL",
		"MONGO_URL",
		"MONGODB_URL",
		"REDIS_URL",
		"CACHE_URL",
		"CONNECTION_STRING",
		"DB_CONNECTION",
		"DB_HOST",
		"DB_PORT",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
		"DB_DATABASE",
		"POSTGRES_HOST",
		"POSTGRES_PORT",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"MYSQL_HOST",
		"MYSQL_PORT",
		"MYSQL_USER",
		"MYSQL_PASSWORD",
		"MYSQL_DATABASE",
		"REDIS_HOST",
		"REDIS_PORT",
		"REDIS_PASSWORD",
		"MONGODB_URI",
		"MONGO_HOST",
		"MONGO_PORT",
		"MONGO_USER",
		"MONGO_PASSWORD",
	}

	// Check each environment variable
	for key, value := range env {
		isDatabase := false

		// Check if key matches any database pattern
		upperKey := strings.ToUpper(key)
		for _, pattern := range databasePatterns {
			if upperKey == pattern || strings.Contains(upperKey, pattern) {
				isDatabase = true
				break
			}
		}

		// Also check for generic password/secret patterns that might be database-related
		if strings.Contains(upperKey, "PASSWORD") ||
			strings.Contains(upperKey, "SECRET") ||
			strings.Contains(upperKey, "_KEY") ||
			strings.Contains(upperKey, "_TOKEN") {
			// Check if it's in a database context
			if strings.Contains(upperKey, "DB") ||
				strings.Contains(upperKey, "DATABASE") ||
				strings.Contains(upperKey, "POSTGRES") ||
				strings.Contains(upperKey, "MYSQL") ||
				strings.Contains(upperKey, "MONGO") ||
				strings.Contains(upperKey, "REDIS") {
				isDatabase = true
			}
		}

		if isDatabase {
			// Skip secrets that will be provided by managed services
			shouldSkip := false

			// Skip DATABASE_URL and PostgreSQL-related secrets if managed Postgres is proposed
			if databaseDesired != DatabaseKindNone {
				if upperKey == "DATABASE_URL" ||
					strings.Contains(upperKey, "POSTGRES") {
					shouldSkip = true
				}
				// Only skip generic DB secrets if they're connection-related (not passwords/keys)
				if strings.Contains(upperKey, "DB") && !strings.Contains(upperKey, "REDIS") {
					if strings.Contains(upperKey, "URL") || strings.Contains(upperKey, "HOST") ||
						strings.Contains(upperKey, "PORT") || strings.Contains(upperKey, "NAME") ||
						strings.Contains(upperKey, "DATABASE") || upperKey == "DB_CONNECTION" {
						shouldSkip = true
					}
				}
			}

			// Skip Redis-related secrets if managed Redis is proposed
			if redisDesired {
				if upperKey == "REDIS_URL" ||
					strings.Contains(upperKey, "REDIS") ||
					upperKey == "CACHE_URL" {
					shouldSkip = true
				}
			}

			if !shouldSkip {
				// Create a secret for this environment variable
				secret := Secret{
					Key:   key,
					Value: value,
					Help:  fmt.Sprintf("Database credential from docker-compose.yml (was: %s)", key),
				}
				secrets = append(secrets, secret)
			}

			// Remove from environment map regardless (managed services will provide these)
			delete(env, key)
		}
	}

	return secrets
}

// processDockerComposeSecrets converts Docker Compose secrets to Fly.io secrets
func processDockerComposeSecrets(secrets map[string]DockerComposeSecret, services map[string]DockerComposeService, sourceDir string) []Secret {
	var flySecrets []Secret

	for secretName, secret := range secrets {
		// Skip external secrets - these need to be managed separately
		if secret.External {
			continue
		}

		var secretValue string
		var helpText string

		if secret.File != "" {
			// Read secret from file
			filePath := filepath.Join(sourceDir, secret.File)
			data, err := os.ReadFile(filePath)
			if err != nil {
				helpText = fmt.Sprintf("Secret from file %s (could not read: %v)", secret.File, err)
				secretValue = "" // Will prompt user during launch
			} else {
				secretValue = strings.TrimSpace(string(data))
				helpText = fmt.Sprintf("Secret from Docker Compose file: %s", secret.File)
			}
		} else {
			helpText = fmt.Sprintf("Docker Compose secret: %s", secretName)
			secretValue = "" // Will prompt user during launch
		}

		flySecret := Secret{
			Key:   secretName,
			Value: secretValue,
			Help:  helpText,
		}
		flySecrets = append(flySecrets, flySecret)
	}

	return flySecrets
}

// extractServiceSecrets extracts secret names from a service's secrets configuration
func extractServiceSecrets(secrets interface{}) []string {
	var secretNames []string

	if secrets == nil {
		return secretNames
	}

	switch s := secrets.(type) {
	case []interface{}:
		// Secrets can be a simple array of strings or array of maps
		for _, secret := range s {
			switch sec := secret.(type) {
			case string:
				// Simple string format: just the secret name
				secretNames = append(secretNames, sec)
			case map[string]interface{}:
				// Map format with source/target
				if source, ok := sec["source"].(string); ok {
					secretNames = append(secretNames, source)
				}
			}
		}
	}

	return secretNames
}

// generateEntrypointScript creates a shell script that sets up /etc/hosts for service discovery
func generateEntrypointScript(containers []Container) []byte {
	script := `#!/bin/sh
set -e

# Add service names to /etc/hosts for multi-container service discovery
# This allows containers to access each other using their service names
# We append to the existing /etc/hosts to preserve Fly.io networking entries

# Only add entries if they don't already exist
`

	// Add each container service name pointing to localhost
	for _, container := range containers {
		script += fmt.Sprintf(`if ! grep -q "\\s%s\\(\\s\\|$\\)" /etc/hosts; then
    echo "127.0.0.1    %s" >> /etc/hosts
fi
`, container.Name, container.Name)
	}

	script += `
# Chain to the original entrypoint or command
if [ $# -eq 0 ]; then
    # No arguments provided - use the default shell
    # This case should be handled by fly.machine.json with proper cmd
    exec /bin/sh
elif [ -x "$1" ]; then
    # First argument is executable, run it directly
    exec "$@"
else
    # First argument is not executable, run it with shell
    exec /bin/sh -c "$*"
fi
`

	return []byte(script)
}

// parseBindMount parses a Docker Compose volume string and returns a ContainerFile for bind mounts
// Handles formats like:
// - "./nginx.conf:/etc/nginx/conf.d/default.conf:ro"
// - "./config:/app/config"
// - "/host/path:/container/path:rw"
func parseBindMount(volumeSpec, sourceDir string) *ContainerFile {
	// Skip named volumes and other non-bind mount formats
	if !strings.Contains(volumeSpec, ":") {
		return nil // Named volume, not a bind mount
	}

	parts := strings.Split(volumeSpec, ":")
	if len(parts) < 2 {
		return nil // Invalid format
	}

	hostPath := parts[0]
	guestPath := parts[1]

	// Only process bind mounts (relative or absolute paths)
	// Skip named volumes (which don't start with . or /)
	if !strings.HasPrefix(hostPath, "./") && !strings.HasPrefix(hostPath, "/") && !strings.HasPrefix(hostPath, "../") {
		return nil // Likely a named volume, not a bind mount
	}

	// Convert relative paths to absolute paths relative to compose file location
	var localPath string
	if strings.HasPrefix(hostPath, "./") || strings.HasPrefix(hostPath, "../") {
		localPath = filepath.Clean(strings.TrimPrefix(hostPath, "./"))
	} else {
		// Absolute path - use as-is but make it relative to sourceDir if possible
		if rel, err := filepath.Rel(sourceDir, hostPath); err == nil && !strings.HasPrefix(rel, "..") {
			localPath = rel
		} else {
			// Can't make it relative, use the basename
			localPath = filepath.Base(hostPath)
		}
	}

	// Determine file mode based on read-only flag
	mode := 0644 // Default to read-write
	if len(parts) >= 3 {
		options := parts[2]
		if strings.Contains(options, "ro") {
			mode = 0444 // Read-only
		}
	}

	return &ContainerFile{
		GuestPath: guestPath,
		LocalPath: localPath,
		Mode:      mode,
	}
}

// mapRestartPolicy maps Docker Compose restart policies to Fly.io restart policies
func mapRestartPolicy(dockerPolicy string) string {
	switch dockerPolicy {
	case "no":
		return "no"
	case "always":
		return "always"
	case "on-failure":
		return "on-failure"
	case "unless-stopped":
		// Docker Compose "unless-stopped" is closest to "always" in Fly.io
		return "always"
	default:
		// Default to "on-failure" for unknown policies
		return "on-failure"
	}
}

// buildConfig represents a build configuration for validating identical build sections
type buildConfig struct {
	context    string
	dockerfile string
	args       map[string]string
	target     string
}

// areBuildConfigsIdentical compares two build configurations to ensure they are identical
func areBuildConfigsIdentical(a, b *buildConfig) bool {
	// Compare basic fields
	if a.context != b.context || a.dockerfile != b.dockerfile || a.target != b.target {
		return false
	}

	// Compare args maps
	if len(a.args) != len(b.args) {
		return false
	}
	for k, v := range a.args {
		if bv, ok := b.args[k]; !ok || v != bv {
			return false
		}
	}

	return true
}
