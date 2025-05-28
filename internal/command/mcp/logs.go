package mcp

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newLogs() *cobra.Command {
	const (
		short = "[experimental] Show log for an MCP server"
		long  = short + "\n"
		usage = "logs"
	)

	cmd := command.New(usage, short, long, runLogs)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.App(),
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file (can be specified multiple times)",
		},
		flag.String{
			Name:        "server",
			Description: "Name of the MCP server to show logs for",
		},
		flag.Bool{
			Name:        "json",
			Description: "Output in JSON format",
		},
		flag.Bool{
			Name:        "no-tail",
			Shorthand:   "n",
			Description: "Do not continually stream logs",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Select MCP server from the " + name + " client configuration",
			},
		)
	}

	return cmd
}

func runLogs(ctx context.Context) error {
	// Get a list of config paths
	configPaths, err := ListConfigPaths(ctx, true)
	if err != nil {
		return err
	}

	if len(configPaths) == 0 {
		return fmt.Errorf("no MCP client configuration files found")
	} else if len(configPaths) > 1 {
		return fmt.Errorf("multiple MCP client configuration files found, please specify one")
	}

	// Get the server name
	name := flag.GetString(ctx, "server")
	if name == "" {
		return fmt.Errorf("please specify a server name")
	}

	server, err := configExtract(configPaths[0], name)
	if err != nil {
		return err
	}

	if app, ok := server["app"]; ok {
		args := []string{"logs", "--app", app.(string)}

		if flag.GetBool(ctx, "json") {
			args = append(args, "--json")
		}

		if flag.GetBool(ctx, "no-tail") {
			args = append(args, "--no-tail")
		}

		if err := flyctl(args...); err != nil {
			return fmt.Errorf("failed to run 'fly logs': %w", err)
		}
	} else {
		return fmt.Errorf("MCP server %s does not have an app", name)
	}

	return nil
}
