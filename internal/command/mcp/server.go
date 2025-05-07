package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	mcpGo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	mcpServer "github.com/superfly/flyctl/internal/command/mcp/server"
	"github.com/superfly/flyctl/internal/flag"
)

var COMMANDS = slices.Concat(
	mcpServer.AppCommands,
	mcpServer.LogCommands,
	mcpServer.MachineCommands,
	mcpServer.OrgCommands,
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

	flag.Add(cmd,
		flag.Bool{
			Name:        "inspector",
			Description: "Launch MCP inspector: a developer tool for testing and debugging MCP servers",
			Default:     false,
			Shorthand:   "i",
		},
	)

	return cmd
}

func runServer(ctx context.Context) error {
	flyctl, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	if flag.GetBool(ctx, "inspector") {
		// Launch MCP inspector
		cmd := exec.Command("npx", "@modelcontextprotocol/inspector", flyctl, "mcp", "server")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to launch MCP inspector: %w", err)
		}
		return nil
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

				switch description.Type {
				case "string":
					if strValue, ok := argValue.(string); ok {
						args[argName] = strValue
					} else {
						return nil, fmt.Errorf("argument %s must be a string", argName)
					}
				case "enum":
					if strValue, ok := argValue.(string); ok {
						if !slices.Contains(description.Enum, strValue) {
							return nil, fmt.Errorf("argument %s must be one of %v", argName, description.Enum)
						}
						args[argName] = strValue
					} else {
						return nil, fmt.Errorf("argument %s must be a string", argName)
					}
				case "array":
					if arrValue, ok := argValue.([]any); ok {
						if len(arrValue) > 0 {
							strArr := make([]string, len(arrValue))
							for i, v := range arrValue {
								if str, ok := v.(string); ok {
									strArr[i] = str
								} else {
									return nil, fmt.Errorf("argument %s must be an array of strings", argName)
								}
							}
							args[argName] = strings.Join(strArr, ",")
						}
					} else {
						return nil, fmt.Errorf("argument %s must be an array of strings", argName)
					}
				case "number":
					if numValue, ok := argValue.(float64); ok {
						args[argName] = strconv.FormatFloat(numValue, 'f', -1, 64)
					} else {
						return nil, fmt.Errorf("argument %s must be a number", argName)
					}
				case "boolean":
					if boolValue, ok := argValue.(bool); ok {
						args[argName] = strconv.FormatBool(boolValue)
					} else {
						return nil, fmt.Errorf("argument %s must be a boolean", argName)
					}
				default:
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

			case "enum":
				if arg.Default != "" {
					if slices.Contains(arg.Enum, arg.Default) {
						options = append(options, mcpGo.DefaultString(arg.Default))
					} else {
						return fmt.Errorf("invalid default value for argument %s: %s is not in enum %v", argName, arg.Default, arg.Enum)
					}
				}

				if len(arg.Enum) > 0 {
					options = append(options, mcpGo.Enum(arg.Enum...))
				} else {
					return fmt.Errorf("enum argument %s must have at least one value", argName)
				}

				toolOptions = append(toolOptions, mcpGo.WithString(argName, options...))

			case "array":
				schema := map[string]any{"type": "string"}
				options = append(options, mcpGo.Items(schema))

				toolOptions = append(toolOptions, mcpGo.WithArray(argName, options...))

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
		return fmt.Errorf("Server error: %v", err)
	}

	return nil
}
