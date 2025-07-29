package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/lib/command"
	"github.com/superfly/flyctl/lib/flag"
)

func newList() *cobra.Command {
	const (
		short = "[experimental] List MCP servers"
		long  = short + "\n"
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.App(),
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file (can be specified multiple times)",
		},
		flag.Bool{
			Name:        "json",
			Description: "Output in JSON format",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "List MCP servers from the " + name + " client configuration",
			},
		)
	}

	return cmd
}

func runList(ctx context.Context) error {
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
		return err
	}

	// build a server map from all of the configs
	serverMap := make(map[string]any)

	for _, configPath := range configPaths {
		// if the configuration file doesn't exist, skip it
		if _, err := os.Stat(configPath.Path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		// read the configuration file
		file, err := os.Open(configPath.Path)
		if err != nil {
			return err
		}
		defer file.Close()

		// parse the configuration file as JSON
		var data map[string]any
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&data); err != nil {
			return fmt.Errorf("failed to parse %s: %w", configPath.Path, err)
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
						return fmt.Errorf("failed to extract config for %s: %w", name, err)
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

	// if the user has specified the --json flag, output the server map as JSON
	if flag.GetBool(ctx, "json") {
		output, err := json.MarshalIndent(serverMap, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal server map: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	// if no MCP servers were found, print a message and return
	if len(serverMap) == 0 {
		fmt.Println("No MCP servers found.")
		return nil
	}

	// print the server map in a human-readable format
	for pathName, configPath := range serverMap {
		fmt.Printf("Config Path: %s\n", pathName)
		if config, ok := configPath.(map[string]any); ok {
			if toolName, ok := config["toolName"].(string); ok {
				fmt.Printf("  Tool Name: %s\n", toolName)
			}

			if servers, ok := config["mcpServers"].(map[string]any); ok {
				for name := range servers {
					fmt.Printf("  MCP Server: %v\n", name)

					server, ok := servers[name].(map[string]any)

					if ok {
						for key, value := range server {
							if key != "command" && key != "args" {
								fmt.Printf("    %s: %v\n", key, value)
							}
						}
					}
				}
			}
		}
		fmt.Println()
	}

	return nil
}
