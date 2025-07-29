package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	mcpGo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/lib/buildinfo"
	"github.com/superfly/flyctl/lib/command"
	mcpServer "github.com/superfly/flyctl/lib/command/mcp/server"
	"github.com/superfly/flyctl/lib/config"
	"github.com/superfly/flyctl/lib/flag"
	"github.com/superfly/flyctl/lib/flag/flagnames"
)

var COMMANDS = slices.Concat(
	mcpServer.AppCommands,
	mcpServer.CertsCommands,
	mcpServer.IPCommands,
	mcpServer.LogCommands,
	mcpServer.MachineCommands,
	mcpServer.OrgCommands,
	mcpServer.PlatformCommands,
	mcpServer.SecretsCommands,
	mcpServer.StatusCommands,
	mcpServer.VolumeCommands,
)

type contextKey string

const authTokenKey contextKey = "authToken"

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
		flag.String{
			Name:        "server",
			Description: "Name to use for the MCP server in the MCP client configuration",
		},
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file (can be specified multiple times)",
		},
		flag.Bool{
			Name:        "stream",
			Description: "Enable HTTP streaming output for MCP commands",
		},
		flag.Bool{
			Name:        "sse",
			Description: "Enable Server-Sent Events (SSE) for MCP commands",
		},
		flag.Int{
			Name:        "port",
			Description: "Port to run the MCP server on (default is 8080)",
			Default:     8080,
		},
		flag.String{
			Name:        flagnames.BindAddr,
			Shorthand:   "b",
			Default:     "127.0.0.1",
			Description: "Local address to bind to",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Add flyctl MCP server to the " + name + " client configuration",
			},
		)
	}

	return cmd
}

func runServer(ctx context.Context) error {
	flyctl, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	configs, err := ListConfigPaths(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to list MCP client configuration paths: %w", err)
	}

	stream := flag.GetBool(ctx, "stream")
	sse := flag.GetBool(ctx, "sse")

	if flag.GetBool(ctx, "inspector") || len(configs) > 0 {
		server := flag.GetString(ctx, "server")
		if server == "" {
			server = "flyctl"
		}

		args := []string{"mcp", "server"}

		if stream || sse {
			args = []string{
				"mcp",
				"proxy",
				"--url",
				fmt.Sprintf("http://localhost:%d", flag.GetInt(ctx, "port")),
			}

			if token := getAccessToken(ctx); token != "" {
				args = append(args, "--bearer-token", token)
			}

			if stream {
				args = append(args, "--stream")
			} else {
				args = append(args, "--sse")
			}
		}

		if len(configs) > 0 {
			for _, config := range configs {
				UpdateConfig(ctx, config.Path, config.ConfigName, server, flyctl, args)
			}
		}

		if flag.GetBool(ctx, "inspector") {
			var process *os.Process

			// If sse or stream, start flyctl mcp server in the background
			if stream || sse {
				args := []string{"mcp", "server", "--port", strconv.Itoa(flag.GetInt(ctx, "port"))}

				if token := getAccessToken(ctx); token != "" {
					args = append(args, "--access-token", token)
				}

				if stream {
					args = append(args, "--stream")
				} else {
					args = append(args, "--sse")
				}

				cmd := exec.Command(flyctl, args...)
				cmd.Env = os.Environ()
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Start(); err != nil {
					return fmt.Errorf("failed to start flyctl mcp server in background: %w", err)
				}

				process = cmd.Process
			}

			// Launch MCP inspector
			args = append([]string{"@modelcontextprotocol/inspector@latest", flyctl}, args...)
			cmd := exec.Command("npx", args...)
			cmd.Env = os.Environ()
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to launch MCP inspector: %w", err)
			}

			if process != nil {
				// Attempt to kill the background process after inspector exits
				if err := process.Kill(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to kill background flyctl mcp server: %v\n", err)
				}
			}
		}

		return nil
	}

	// Create MCP server
	srv := server.NewMCPServer(
		"FlyMCP 🚀",
		buildinfo.Info().Version.String(),
	)

	// Register commands
	for _, cmd := range COMMANDS {
		// Create a tool function for each command
		tool := func(ctx context.Context, request mcpGo.CallToolRequest) (*mcpGo.CallToolResult, error) {
			// Extract arguments from the request
			args := make(map[string]string)
			argMap, ok := request.Params.Arguments.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid arguments: expected map[string]any")
			}
			for argName, argValue := range argMap {
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
				case "hash":
					if arrValue, ok := argValue.([]any); ok {
						if len(arrValue) > 0 {
							strArr := make([]string, len(arrValue))
							for i, v := range arrValue {
								if str, ok := v.(string); ok {
									// Simple shell escaping: wrap value in single quotes and escape any single quotes inside
									str = "'" + strings.ReplaceAll(str, "'", "'\\''") + "'"
									strArr[i] = str
								} else {
									return nil, fmt.Errorf("argument %s must be an array of strings", argName)
								}
							}
							args[argName] = strings.Join(strArr, " ")
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

			// Log the command (without the auth token and any secret values)
			if len(cmdArgs) >= 2 && cmdArgs[0] == "secrets" && cmdArgs[1] == "set" {
				redactedCmdArgs := append([]string(nil), cmdArgs...)
				for i, arg := range redactedCmdArgs[2:] {
					if strings.Contains(arg, "=") {
						parts := strings.SplitN(arg, "=", 2)
						redactedCmdArgs[i+2] = parts[0] + "=REDACTED"
					}
				}
				fmt.Fprintf(os.Stderr, "Executing flyctl command: %v\n", redactedCmdArgs)
			} else {
				fmt.Fprintf(os.Stderr, "Executing flyctl command: %v\n", cmdArgs)
			}

			// If auth token is present in context, add --access-token flag
			if token, ok := ctx.Value(authTokenKey).(string); ok && token != "" {
				cmdArgs = append(cmdArgs, "--access-token", token)
			}

			// Execute the command
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

			case "hash":
				schema := map[string]any{"type": "string"}
				options = append(options, mcpGo.Items(schema))

				toolOptions = append(toolOptions, mcpGo.WithArray(argName, options...))

			default:
				return fmt.Errorf("unsupported argument type %s for argument %s", arg.Type, argName)
			}
		}

		srv.AddTool(
			mcpGo.NewTool(cmd.ToolName, toolOptions...),
			tool,
		)
	}

	if defaultToken := getAccessToken(ctx); defaultToken != "" {
		ctx = context.WithValue(ctx, authTokenKey, defaultToken)
	}

	if stream || sse {
		port := flag.GetInt(ctx, "port")
		var start func(string) error
		var err error

		// enable graceful shutdown on signals
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc,
			syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT)
		go func() {
			<-sigc
			os.Exit(0)
		}()

		// Function to extract the auth token from the request context
		extractAuthToken := func(ctx context.Context, r *http.Request) context.Context {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				// Extract the token from the Authorization header
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != authHeader { // Ensure it was a Bearer token
					return context.WithValue(ctx, authTokenKey, token)
				}
			}

			return ctx
		}

		if stream {
			fmt.Fprintf(os.Stderr, "Starting flyctl MCP server in streaming mode on port %d...\n", port)
			httpServer := server.NewStreamableHTTPServer(srv)
			server.WithHTTPContextFunc(extractAuthToken)(httpServer)
			start = httpServer.Start
		} else {
			fmt.Fprintf(os.Stderr, "Starting flyctl MCP server in SSE mode on port %d...\n", port)
			sseServer := server.NewSSEServer(srv)
			server.WithSSEContextFunc(extractAuthToken)(sseServer)
			start = sseServer.Start
		}

		if err = start(fmt.Sprintf("%s:%d", flag.GetString(ctx, flagnames.BindAddr), port)); err != nil {
			return fmt.Errorf("Server error: %v", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Starting flyctl MCP server...\n")
		if err := server.ServeStdio(srv); err != nil {
			return fmt.Errorf("Server error: %v", err)
		}
	}

	return nil
}

func getAccessToken(ctx context.Context) string {
	token := flag.GetString(ctx, flagnames.AccessToken)

	if token == "" {
		cfg := config.FromContext(ctx)
		token = cfg.Tokens.GraphQL()
	}

	return token
}
