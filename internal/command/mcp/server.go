package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"

	mcpGo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	mcpServer "github.com/superfly/flyctl/internal/command/mcp/server"
)

var COMMANDS = slices.Concat(
	mcpServer.LogCommands,
	mcpServer.StatusCommands,
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

				if strValue, ok := argValue.(string); ok {
					args[argName] = strValue
				} else {
					return nil, fmt.Errorf("argument %s must be a string", argName)
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
				toolOptions = append(toolOptions, mcpGo.WithString(argName, options...))
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
