package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

// FlyCommand represents a command for the Fly MCP server
type FlyCommand struct {
	ToolName        string
	ToolDescription string
	ToolArgs        map[string]FlyArg
	Builder         func(args map[string]string) ([]string, error)
}

// FlyArg represents an argument for a Fly command
type FlyArg struct {
	Description string
	Required    bool
	Type        string
}

var COMMANDS = []FlyCommand{
	{
		ToolName:        "fly-logs",
		ToolDescription: "Get logs for a Fly.io app or specific machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"machine": {
				Description: "Specific machine ID",
				Required:    false,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"logs", "--no-tail"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if machine, ok := args["machine"]; ok {
				cmdArgs = append(cmdArgs, "--machine", machine)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-status",
		ToolDescription: "Get status of a Fly.io app",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"status", "--json"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			return cmdArgs, nil
		},
	},
}

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
		tool := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			return mcp.NewToolResultText(string(output)), nil
		}

		// Register the tool with the server
		toolOptions := []mcp.ToolOption{
			mcp.WithDescription(cmd.ToolDescription),
		}

		for argName, arg := range cmd.ToolArgs {
			options := []mcp.PropertyOption{
				mcp.Description(arg.Description),
			}

			if arg.Required {
				options = append(options, mcp.Required())
			}

			switch arg.Type {
			case "string":
				toolOptions = append(toolOptions, mcp.WithString(argName, options...))
			default:
				return fmt.Errorf("unsupported argument type %s for argument %s", arg.Type, argName)
			}
		}

		srv.AddTool(
			mcp.NewTool(cmd.ToolName, toolOptions...),
			tool,
		)
	}

	fmt.Fprintf(os.Stderr, "Starting MCP server...\n")
	if err := server.ServeStdio(srv); err != nil {
		return fmt.Errorf("Server error: %v\n", err)
	}

	return nil
}
