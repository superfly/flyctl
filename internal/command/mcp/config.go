package mcp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/prompt"

	"github.com/superfly/fly-go/flaps"
)

var McpClients = map[string]string{
	"claude":   "Claude",
	"vscode":   "VS Code",
	"cursor":   "Cursor",
	"neovim":   "Neovim",
	"windsurf": "Windsurf",
	"zed":      "Zed",
}

// ConfigPath represents a configuration file path
type ConfigPath struct {
	ToolName   string
	Path       string
	ConfigName string
}

func NewAdd() *cobra.Command {
	const (
		short = "[experimental] Add MCP proxy client to a MCP client configuration"
		long  = short + "\n"
		usage = "add"
	)

	cmd := command.New(usage, short, long, runAdd, command.RequireAppName)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.App(),
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file (can be specified multiple times)",
		},
		flag.String{
			Name:        "url",
			Description: "URL of the MCP wrapper server",
		},
		flag.String{
			Name:        "name",
			Description: "Name to use for the MCP server in the MCP client configuration",
			Hidden:      true,
		},
		flag.String{
			Name:        "server",
			Description: "Name to use for the MCP server in the MCP client configuration",
		},
		flag.String{
			Name:        "user",
			Description: "User to authenticate with",
		},
		flag.String{
			Name:        "password",
			Description: "Password to authenticate with",
		},
		flag.Bool{
			Name:        "bearer-token",
			Description: "Use bearer token for authentication",
			Default:     true,
		},
		flag.Bool{
			Name:        "flycast",
			Description: "Use wireguard and flycast for access",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Add MCP server to the " + name + " client configuration",
			},
		)
	}

	return cmd
}

func NewRemove() *cobra.Command {
	const (
		short = "[experimental] Remove MCP proxy client from a MCP client configuration"
		long  = short + "\n"
		usage = "remove"
	)
	cmd := command.New(usage, short, long, runRemove, command.LoadAppNameIfPresent)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.App(),
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file (can be specified multiple times)",
		},
		flag.String{
			Name:        "name",
			Description: "Name to use for the MCP server in the MCP client configuration",
			Hidden:      true,
		},
		flag.String{
			Name:        "server",
			Description: "Name to use for the MCP server in the MCP client configuration",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Remove MCP server from the " + name + " client configuration",
			},
		)
	}
	return cmd
}

func runAdd(ctx context.Context) error {
	appConfig := appconfig.ConfigFromContext(ctx)
	if appConfig == nil {
		appName := appconfig.NameFromContext(ctx)
		if appName == "" {
			return errors.New("app name is required")
		} else {
			// Set up flaps client in context before calling FromRemoteApp
			if flapsutil.ClientFromContext(ctx) == nil {
				flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
					AppName: appName,
				})
				if err != nil {
					return fmt.Errorf("could not create flaps client: %w", err)
				}
				ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
			}

			var err error
			appConfig, err = appconfig.FromRemoteApp(ctx, appName)
			if err != nil {
				return err
			}
		}
	}

	url := flag.GetString(ctx, "url")
	if url == "" {
		if flag.GetBool(ctx, "flycast") {
			url = "http://" + appConfig.AppName + ".flycast/"
		} else {
			appUrl := appConfig.URL()
			if appUrl == nil {
				return errors.New("The app doesn't expose a public http service")
			}
			url = appUrl.String()
		}
	}

	args := []string{"mcp", "proxy", "--url", url}

	user := flag.GetString(ctx, "user")
	if user != "" {
		args = append(args, "--user", user)

		password := flag.GetString(ctx, "password")
		if password == "" {
			err := prompt.Password(ctx, &password, "Password:", true)
			if err != nil && !prompt.IsNonInteractive(err) {
				return fmt.Errorf("failed to get password: %w", err)
			}
			args = append(args, "--password", password)
		}

		if err := flyctl("secrets", "set", "FLY_MCP_USER="+user, "FLY_MCP_PASSWORD="+password, "--app", appConfig.AppName); err != nil {
			return fmt.Errorf("failed to set user/password secrets': %w", err)
		}

	} else if flag.GetBool(ctx, "bearer-token") {
		// Generate a secure random 24 character base64 encoded string for bearerToken
		b := make([]byte, 18) // 18 bytes = 24 base64 characters
		_, err := rand.Read(b)
		if err != nil {
			return fmt.Errorf("failed to generate bearer token: %w", err)
		}
		bearerTokenStr := base64.StdEncoding.EncodeToString(b)
		args = append(args, "--bearer-token", bearerTokenStr)

		if err := flyctl("secrets", "set", "FLY_MCP_BEARER_TOKEN="+bearerTokenStr, "--app", appConfig.AppName); err != nil {
			return fmt.Errorf("failed to set bearer token secret': %w", err)
		}
	}

	configPaths, err := ListConfigPaths(ctx, true)
	if err != nil {
		return err
	} else if len(configPaths) == 0 {
		return errors.New("no configuration paths found")
	}

	server := flag.GetString(ctx, "server")
	if server == "" {
		server = flag.GetString(ctx, "name")
		if server == "" {
			server = appConfig.AppName
		}
	}

	flyctlExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	for _, configPath := range configPaths {
		err = UpdateConfig(ctx, configPath.Path, configPath.ConfigName, server, flyctlExecutable, args)
		if err != nil {
			return fmt.Errorf("failed to update configuration at %s: %w", configPath.Path, err)
		}
	}

	return nil
}

// Build a list of configuration paths to update
func ListConfigPaths(ctx context.Context, configIsArray bool) ([]ConfigPath, error) {
	log := logger.FromContext(ctx)

	var paths []ConfigPath

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// OS-specific paths
	var configDir string
	if runtime.GOOS == "darwin" {
		configDir = filepath.Join(home, "Library", "Application Support")
	} else if runtime.GOOS == "windows" {
		configDir = filepath.Join(home, "AppData", "Roaming")
	} else {
		configDir = filepath.Join(home, ".config")
	}

	// Claude configuration
	if flag.GetBool(ctx, "claude") {
		claudePath := filepath.Join(configDir, "Claude", "claude_desktop_config.json")
		log.Debugf("Adding Claude configuration path: %s", claudePath)
		paths = append(paths, ConfigPath{ToolName: "claude", Path: claudePath})
	}

	// VS Code configuration
	if flag.GetBool(ctx, "vscode") {
		vscodePath := filepath.Join(configDir, "Code", "User", "settings.json")
		log.Debugf("Adding VS Code configuration path: %s", vscodePath)
		paths = append(paths, ConfigPath{ToolName: "vscode", Path: vscodePath, ConfigName: "mcp"})
	}

	// Cursor configuration
	if flag.GetBool(ctx, "cursor") {
		cursorPath := filepath.Join(configDir, "Cursor", "config.json")
		log.Debugf("Adding Cursor configuration path: %s", cursorPath)
		paths = append(paths, ConfigPath{ToolName: "cursor", Path: cursorPath})
	}

	// Neovim configuration
	if flag.GetBool(ctx, "neovim") {
		neovimPath := filepath.Join(configDir, "nvim", "init.json")
		log.Debugf("Adding Neovim configuration path: %s", neovimPath)
		paths = append(paths, ConfigPath{ToolName: "neovim", Path: neovimPath})
	}

	// Windsurf configuration
	if flag.GetBool(ctx, "windsurf") {
		windsurfPath := filepath.Join(home, ".codeium", "windsurf", "config.json")
		log.Debugf("Adding Windsurf configuration path: %s", windsurfPath)
		paths = append(paths, ConfigPath{ToolName: "windsurf", Path: windsurfPath})
	}

	// Zed configuration
	if flag.GetBool(ctx, "zed") {
		zedPath := filepath.Join(home, ".config", "zed", "settings.json")
		log.Debugf("Adding Zed configuration path: %s", zedPath)
		paths = append(paths, ConfigPath{ToolName: "zed", Path: zedPath, ConfigName: "context_servers"})
	}

	if configIsArray {
		// Add custom configuration paths
		for _, path := range flag.GetStringArray(ctx, "config") {
			path, err := filepath.Abs(path)
			if err != nil {
				return nil, fmt.Errorf("failed to get absolute path for %s: %w", path, err)
			}
			log.Debugf("Adding custom configuration path: %s", path)
			paths = append(paths, ConfigPath{Path: path})
		}
	} else {
		path := flag.GetString(ctx, "config")
		if path != "" {
			path, err := filepath.Abs(path)
			if err != nil {
				return nil, fmt.Errorf("failed to get absolute path for %s: %w", path, err)
			}
			log.Debugf("Adding custom configuration path: %s", path)
			paths = append(paths, ConfigPath{Path: path})
		}
	}

	for i, configPath := range paths {
		if configPath.ConfigName == "" {
			paths[i].ConfigName = "mcpServers"
		}
	}

	return paths, nil
}

func ServerMap(configPaths []ConfigPath) (map[string]any, error) {
	// build a server map from all of the configs
	serverMap := make(map[string]any)

	for _, configPath := range configPaths {
		// if the configuration file doesn't exist, skip it
		if _, err := os.Stat(configPath.Path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		// read the configuration file
		file, err := os.Open(configPath.Path)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		// parse the configuration file as JSON
		var data map[string]any
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&data); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", configPath.Path, err)
		}

		if mcpServers, ok := data[configPath.ConfigName].(map[string]any); ok {
			// add metadata about the tool
			config := make(map[string]any)
			config["mcpServers"] = mcpServers
			config["configName"] = configPath.ConfigName

			if configPath.ToolName != "" {
				config["toolName"] = configPath.ToolName
			}

			serverMap[configPath.Path] = config

			// add metadata about each MCP server
			for name := range mcpServers {
				if serverMap, ok := mcpServers[name].(map[string]any); ok {
					server, err := configExtract(configPath, name)
					if err != nil {
						return nil, fmt.Errorf("failed to extract config for %s: %w", name, err)
					}

					for key, value := range server {
						if key != "command" && key != "args" {
							serverMap[key] = value
						}
					}

					mcpServers[name] = serverMap
				}
			}
		}
	}

	return serverMap, nil
}

func SelectServerAndConfig(ctx context.Context, configIsArray bool) (string, []ConfigPath, error) {
	server := flag.GetString(ctx, "server")

	// Check if the user has specified any client flags
	configSelected := false
	for client := range McpClients {
		configSelected = configSelected || flag.GetBool(ctx, client)
	}

	// if no cllent is selected, select all clients
	if !configSelected {
		for client := range McpClients {
			flag.SetString(ctx, client, "true")
		}
	}

	// Get a list of config paths
	configPaths, err := ListConfigPaths(ctx, true)
	if err != nil {
		return "", nil, err
	}

	var serverMap map[string]any

	if len(configPaths) > 1 || server == "" {
		serverMap, err = ServerMap(configPaths)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get server map: %w", err)
		}
	}

	if len(configPaths) == 0 {
		return "", nil, errors.New("no configuration paths found")
	} else if len(configPaths) > 1 && !configIsArray {
		choices := make([]string, 0)
		choiceMap := make(map[int]int)
		for i, configPath := range configPaths {
			if config, ok := serverMap[configPath.Path].(map[string]any); ok {
				if servers, ok := config["mcpServers"].(map[string]any); ok && len(servers) > 0 {
					if toolName, ok := config["toolName"].(string); ok {
						choices = append(choices, toolName)
					} else {
						choices = append(choices, configPath.Path)
					}
					choiceMap[i] = len(choices) - 1
				}
			}
		}

		index := 0
		if len(choices) == 0 {
			return "", nil, errors.New("no MCP servers found in the selected configuration files")
		} else if len(choices) > 1 {
			switch err = prompt.Select(ctx, &index, "Select a configuration file", "", choices...); {
			case err == nil:
				if choiceIndex, ok := choiceMap[index]; ok {
					index = choiceIndex
				}
			case prompt.IsNonInteractive(err):
				return "", nil, prompt.NonInteractiveError("MCP client or config file must be specified when not running interactively")
			default:
				return "", nil, fmt.Errorf("failed to select configuration file: %w", err)
			}

		}

		configPaths = []ConfigPath{configPaths[index]}
	}

	if server == "" {
		if len(serverMap) == 0 {
			return "", configPaths, errors.New("no MCP servers found in the selected configuration files")
		}
		// Select a server from the server map
		var index int
		choices := make([]string, 0)
		for _, configPath := range serverMap {
			if config, ok := configPath.(map[string]any); ok {
				if servers, ok := config["mcpServers"].(map[string]any); ok {
					for name := range servers {
						choices = append(choices, name)
					}
				}
			}
		}

		if len(choices) == 0 {
			return "", configPaths, errors.New("no MCP servers found in the selected configuration files")
		} else if len(choices) == 1 {
			server = choices[0]
			log.Debugf("Only one MCP server found: %s", server)
		} else {
			switch err = prompt.Select(ctx, &index, "Select a MCP server", "", choices...); {
			case err == nil:
				server = choices[index]
				log.Debugf("Selected MCP server: %s", server)
			case prompt.IsNonInteractive(err):
				return "", nil, prompt.NonInteractiveError("server must be specified when not running interactively")
			default:
				return "", configPaths, fmt.Errorf("failed to select MCP server: %w", err)
			}
		}
	}

	return server, configPaths, nil
}

// UpdateConfig updates the configuration at the specified path with the MCP servers
func UpdateConfig(ctx context.Context, path string, configKey string, server string, command string, args []string) error {
	log.Debugf("Updating configuration at %s", path)

	if configKey == "" {
		configKey = "mcpServers"
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Initialize configuration data map
	configData := make(map[string]interface{})

	// Read existing configuration if it exists
	fileExists := false
	fileData, err := os.ReadFile(path)
	if err == nil {
		fileExists = true
		// File exists, parse it
		err = json.Unmarshal(fileData, &configData)
		if err != nil {
			return fmt.Errorf("failed to parse existing configuration at %s: %w", path, err)
		} else {
			log.Debugf("Successfully read existing configuration at %s", path)
		}
	} else {
		log.Debugf("Configuration file doesn't exist, will create a new one")
	}

	// Get or create mcpServers field in config
	var mcpServers map[string]interface{}

	if mcpServersRaw, exists := configData[configKey]; exists {
		if mcpMap, ok := mcpServersRaw.(map[string]interface{}); ok {
			mcpServers = mcpMap
			log.Debugf("Found existing %s with %d entries", configKey, len(mcpServers))
		} else {
			return fmt.Errorf("%s field exists in %s but is not a map", configKey, path)
		}
	} else {
		log.Debugf("No %s field found, creating a new one", configKey)
		mcpServers = make(map[string]interface{})
	}

	// Merge the new MCP server with existing ones
	if _, exists := mcpServers[server]; exists {
		log.Debugf("Replacing existing MCP server: %s", server)
	} else {
		log.Debugf("Adding new MCP server: %s", server)
	}

	// Build the server map
	serverMap := map[string]interface{}{
		"command": command,
		"args":    args,
	}

	// Update the server in the existing map
	mcpServers[server] = serverMap

	// Update the mcpServers field in the config
	configData[configKey] = mcpServers

	// Write the updated configuration
	updatedData, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated configuration: %w", err)
	}

	err = os.WriteFile(path, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("Failed to write updated configuration to %s: %v", path, err)
	}

	if fileExists {
		log.Debugf("Successfully updated existing configuration at %s", path)
	} else {
		log.Debugf("Successfully created new configuration at %s", path)
	}

	return nil
}

func runRemove(ctx context.Context) error {
	var err error

	server, configPaths, err := SelectServerAndConfig(ctx, false)
	if err != nil {
		return err
	}

	for _, configPath := range configPaths {
		err = removeConfig(ctx, configPath.Path, configPath.ConfigName, server)
		if err != nil {
			return fmt.Errorf("failed to update configuration at %s: %w", configPath.Path, err)
		}
	}

	return nil
}

// removeConfig removes the MCP server from the configuration at the specified path
func removeConfig(ctx context.Context, path string, configKey string, name string) error {
	log := logger.FromContext(ctx)

	log.Debugf("Removing from configuration at %s", path)

	// Read existing configuration if it exists
	fileData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read configuration at %s: %w", path, err)
	}

	// Parse the existing configuration
	configData := make(map[string]interface{})
	err = json.Unmarshal(fileData, &configData)
	if err != nil {
		return fmt.Errorf("failed to parse existing configuration at %s: %w", path, err)
	} else {
		log.Debugf("Successfully read existing configuration at %s", path)
	}

	// Get the mcpServers field in config
	var mcpServers map[string]interface{}
	if mcpServersRaw, exists := configData[configKey]; exists {
		if mcpMap, ok := mcpServersRaw.(map[string]interface{}); ok {
			mcpServers = mcpMap
			log.Debugf("Found existing %s with %d entries", configKey, len(mcpServers))
		} else {
			return fmt.Errorf("%s field exists in %s but is not a map", configKey, path)
		}
	} else {
		log.Warnf("No %s field found, nothing to remove", configKey)
		return nil
	}

	// Remove the MCP server from the existing map
	if _, exists := mcpServers[name]; exists {
		log.Debugf("Removing existing MCP server: %s", name)
		delete(mcpServers, name)
	} else {
		log.Warnf("MCP server %s not found, nothing to remove", name)
		return nil
	}

	// Update the mcpServers field in the config
	configData[configKey] = mcpServers

	// Write the updated configuration
	updatedData, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated configuration: %w", err)
	}

	err = os.WriteFile(path, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("Failed to write updated configuration to %s: %v", path, err)
	}

	log.Debugf("Successfully updated existing configuration at %s", path)
	return nil
}

// Server represents a server configuration in the JSON file
type MCPServer struct {
	Args    []string `json:"args"`
	Command string   `json:"command"`
}

func configExtract(config ConfigPath, server string) (map[string]interface{}, error) {
	// Check if the file exists
	// Read the configuration file
	data, err := os.ReadFile(config.Path)
	if err != nil {
		return nil, fmt.Errorf("Error reading file: %v", err)
	}

	// Parse the JSON data
	jsonConfig := make(map[string]interface{})
	if err := json.Unmarshal(data, &jsonConfig); err != nil {
		return nil, fmt.Errorf("Error parsing JSON: %v", err)
	}

	jsonServers, ok := jsonConfig[config.ConfigName].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Error finding MCP server configuration: %v", err)
	}

	serverConfig, ok := jsonServers[server].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Error finding MCP server configuration: %v", err)
	}

	args, ok := serverConfig["args"].([]interface{})

	if ok {
		for i, arg := range args {
			if arg == "--bearer-token" && i+1 < len(args) {
				serverConfig["bearer-token"] = args[i+1]
			}

			if arg == "--url" && i+1 < len(args) {
				appUrl := args[i+1]
				serverConfig["url"] = appUrl

				if appUrlStr, ok := appUrl.(string); ok {
					parsedURL, err := url.Parse(appUrlStr)
					if err == nil {
						hostnameParts := strings.Split(parsedURL.Hostname(), ".")
						if len(hostnameParts) > 2 && hostnameParts[len(hostnameParts)-1] == "dev" && hostnameParts[len(hostnameParts)-2] == "fly" {
							serverConfig["app"] = hostnameParts[len(hostnameParts)-3]
						} else if len(hostnameParts) > 1 && hostnameParts[len(hostnameParts)-1] == "flycast" {
							serverConfig["app"] = hostnameParts[len(hostnameParts)-2]
						} else if len(hostnameParts) > 1 && hostnameParts[len(hostnameParts)-1] == "internal" {
							serverConfig["app"] = hostnameParts[len(hostnameParts)-2]
						}
					}
				}
			}
		}
	}

	return serverConfig, nil
}
