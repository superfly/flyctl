package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func NewDestroy() *cobra.Command {
	const (
		short = "[experimental] Destroy an MCP stdio server"
		long  = short + "\n"
		usage = "destroy"
	)

	cmd := command.New(usage, short, long, runDestroy, command.LoadAppNameIfPresent)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.App(),

		flag.String{
			Name:        "server",
			Description: "Name of the MCP server in the MCP client configuration",
		},
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file",
		},
		flag.Bool{
			Name:        "yes",
			Description: "Accept all confirmations",
			Shorthand:   "y",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Remove MCP server from to the " + name + " client configuration",
			},
		)
	}

	return cmd
}

func runDestroy(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	if appName == "" {
		server, configPaths, err := SelectServerAndConfig(ctx, true)
		if err != nil {
			return err
		}

		if len(configPaths) == 0 {
			return fmt.Errorf("No app name or MCP client configuration file provided")
		}

		mcpConfig, err := configExtract(configPaths[0], server)
		if err != nil {
			return err
		}

		var ok bool
		appName, ok = mcpConfig["app"].(string)
		if !ok {
			return fmt.Errorf("No app name found in MCP client configuration")
		}
	}

	client := flyutil.ClientFromContext(ctx)
	_, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("app not found: %w", err)
	}

	// Destroy the app
	args := []string{"apps", "destroy", appName}

	if flag.GetBool(ctx, "yes") {
		args = append(args, "--yes")
	}

	if err := flyctl(args...); err != nil {
		return fmt.Errorf("failed to destroy app': %w", err)
	}

	_, err = client.GetApp(ctx, appName)
	if err == nil {
		return fmt.Errorf("app not destroyed: %s", appName)
	}

	args = []string{}

	// Remove the MCP server to the MCP client configurations
	for client := range McpClients {
		if flag.GetBool(ctx, client) {
			args = append(args, "--"+client)
		}
	}

	for _, config := range flag.GetStringArray(ctx, "config") {
		if config != "" {
			log.Debugf("Removing %s from the MCP client configuration", config)
			args = append(args, "--config", config)
		}
	}

	if len(args) > 0 {
		args = append([]string{"mcp", "remove"}, args...)

		if app := flag.GetString(ctx, "app"); app != "" {
			args = append(args, "--app", app)
		}
		if server := flag.GetString(ctx, "server"); server != "" {
			args = append(args, "--server", server)
		}

		// Run 'fly mcp remove ...'
		if err := flyctl(args...); err != nil {
			return fmt.Errorf("failed to run 'fly mcp remove': %w", err)
		}

		log.Debug(strings.Join(args, " "))
	}

	return nil
}
