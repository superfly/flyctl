package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"

	mcpGo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	mcpServer "github.com/superfly/flyctl/internal/command/mcp/server"
)

var COMMANDS = slices.Concat(
	mcpServer.LogCommands,
	mcpServer.PlatformCommands,
	mcpServer.StatusCommands,
	mcpServer.VolumeCommands,
)

func newServer() *cobra.Command {
	const (
		short = "[experimental] Start a flyctl MCP server"
		long  = short + "\n"
		usage = "server"
	)

	cmd := command.New(usage, short, long, runServer)
	cmd.Args = cobra.ExactArgs(0)

	return cmd
}

func runServer(ctx context.Context) error {
	flyctl, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	// Create MCP server
	srv := server.NewMCPServer(
		"FlyMCP 🚀",
		"1.0.0",
	)

	// Register commands
	for _, cmd := range COMMANDS {
		// Create a tool function for each command
		tool := func(ctx context.Context, request mcpGo.CallToolRequest) (*mcpGo.CallToolResult, error) {
			// Extract arguments from the request
			args := make(map[string]string)
			for argName, argValue := range request.Params.Arguments {
				description, ok := cmd.ToolArgs[argName]
				if !ok {
					return nil, fmt.Errorf("unknown argument %s", argName)
				}

				if description.Required && argValue == nil {
					return nil, fmt.Errorf("argument %s is required", argName)
				}

				if description.Type == "string" {
					if strValue, ok := argValue.(string); ok {
						args[argName] = strValue
					} else {
						return nil, fmt.Errorf("argument %s must be a string", argName)
					}
				} else if description.Type == "number" {
					if numValue, ok := argValue.(float64); ok {
						args[argName] = strconv.FormatFloat(numValue, 'f', -1, 64)
					} else {
						return nil, fmt.Errorf("argument %s must be a number", argName)
					}
				} else if description.Type == "boolean" {
					if boolValue, ok := argValue.(bool); ok {
						args[argName] = strconv.FormatBool(boolValue)
					} else {
						return nil, fmt.Errorf("argument %s must be a boolean", argName)
					}
				} else {
					return nil, fmt.Errorf("unsupported argument type %s for argument %s", description.Type, argName)
				}
			}

			// Check for required arguments and fill in defaults
			for argName, description := range cmd.ToolArgs {
				if description.Required {
					if _, ok := args[argName]; !ok {
						return nil, fmt.Errorf("missing required argument %s", argName)
					}
				} else if description.Default != "" {
					if _, ok := args[argName]; !ok {
						args[argName] = description.Default
					}
				}
			}

			// Call the builder function to get the command arguments
			cmdArgs, err := cmd.Builder(args)
			if err != nil {
				return nil, fmt.Errorf("failed to build command: %w", err)
			}

			// Execute the command
			fmt.Fprintf(os.Stderr, "Executing flyctl command: %v\n", cmdArgs)
			execCmd := exec.Command(flyctl, cmdArgs...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error executing flyctl: %v\nOutput: %s\n", err, string(output))
				return nil, fmt.Errorf("failed to execute command: %v\nOutput: %s", err, string(output))
			}

			// Return the output as a tool result
			return mcpGo.NewToolResultText(string(output)), nil
		}

		// Register the tool with the server
		toolOptions := []mcpGo.ToolOption{
			mcpGo.WithDescription(cmd.ToolDescription),
		}

		for argName, arg := range cmd.ToolArgs {
			options := []mcpGo.PropertyOption{
				mcpGo.Description(arg.Description),
			}

			if arg.Required {
				options = append(options, mcpGo.Required())
			}

			switch arg.Type {
			case "string":
				if arg.Default != "" {
					options = append(options, mcpGo.DefaultString(arg.Default))
				}

				toolOptions = append(toolOptions, mcpGo.WithString(argName, options...))

			case "number":
				if arg.Default != "" {
					if defaultValue, err := strconv.ParseFloat(arg.Default, 64); err == nil {
						options = append(options, mcpGo.DefaultNumber(defaultValue))
					} else {
						return fmt.Errorf("invalid default value for argument %s: %v", argName, err)
					}
				}

				toolOptions = append(toolOptions, mcpGo.WithNumber(argName, options...))

			case "boolean":
				if arg.Default != "" {
					if defaultValue, err := strconv.ParseBool(arg.Default); err == nil {
						options = append(options, mcpGo.DefaultBool(defaultValue))
					} else {
						return fmt.Errorf("invalid default value for argument %s: %v", argName, err)
					}
				}

				toolOptions = append(toolOptions, mcpGo.WithBoolean(argName, options...))

			default:
				return fmt.Errorf("unsupported argument type %s for argument %s", arg.Type, argName)
			}
		}

		srv.AddTool(
			mcpGo.NewTool(cmd.ToolName, toolOptions...),
			tool,
		)
	}

	fmt.Fprintf(os.Stderr, "Starting MCP server...\n")
	if err := server.ServeStdio(srv); err != nil {
		return fmt.Errorf("Server error: %v\n", err)
	}

	return nil
}
