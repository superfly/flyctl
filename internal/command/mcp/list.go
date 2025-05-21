package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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

	configPaths, err := ListConfigPaths(ctx, true)
	if err != nil {
		return err
	}

	serverMap := make(map[string]interface{})

	for _, configPath := range configPaths {
		if _, err := os.Stat(configPath.Path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		file, err := os.Open(configPath.Path)
		if err != nil {
			return err
		}
		defer file.Close()

		var data map[string]interface{}
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&data); err != nil {
			return fmt.Errorf("failed to parse %s: %w", configPath.Path, err)
		}

		if val, ok := data[configPath.ConfigName]; ok {
			server := make(map[string]interface{})
			server["mcpServers"] = val
			server["configName"] = configPath.ConfigName

			if configPath.ToolName != "" {
				server["toolName"] = configPath.ToolName
			}

			serverMap[configPath.Path] = server
		}
	}

	if flag.GetBool(ctx, "json") {
		output, err := json.MarshalIndent(serverMap, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal server map: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	if len(serverMap) == 0 {
		fmt.Println("No MCP servers found.")
		return nil
	}

	for pathName, configPath := range serverMap {
		fmt.Printf("Config Path: %s\n", pathName)
		if config, ok := configPath.(map[string]interface{}); ok {
			if servers, ok := config["mcpServers"].(map[string]interface{}); ok {
				for name := range servers {
					fmt.Printf("  MCP Server: %v\n", name)

					configName, ok := config["configName"].(string)

					if ok {
						server, err := configExtract(ConfigPath{Path: pathName, ConfigName: configName}, name)
						if err == nil {
							for key, value := range server {
								if key != "command" && key != "args" {
									fmt.Printf("    %s: %v\n", key, value)
								}
							}
						} else {
							fmt.Printf("    Error: %v\n", err)
						}
					}
				}
			}
		}
		fmt.Println()
	}

	return nil
}
