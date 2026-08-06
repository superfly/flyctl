package mcpServer

import (
	"fmt"
)

var CertsCommands = []FlyCommand{
	{
		ToolName:        "fly-certs-add",
		ToolDescription: "Add an SSL/TLS certificate for a Fly.io app.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"hostname": {
				Description: "The hostname to add a certificate for (e.g. www.example.com)",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"certs", "add", "--json"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			hostname, ok := args["hostname"]
			if !ok || hostname == "" {
				return nil, fmt.Errorf("missing required argument: hostname")
			}
			cmdArgs = append(cmdArgs, hostname)

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-certs-check",
		ToolDescription: "Check the status of an SSL/TLS certificate for a Fly.io app.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"hostname": {
				Description: "The hostname to check the certificate for (e.g. www.example.com)",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"certs", "check", "--json"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			hostname, ok := args["hostname"]
			if !ok || hostname == "" {
				return nil, fmt.Errorf("missing required argument: hostname")
			}
			cmdArgs = append(cmdArgs, hostname)

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-certs-list",
		ToolDescription: "List all SSL/TLS certificates for a Fly.io app.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"certs", "list", "--json"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-certs-remove",
		ToolDescription: "Remove an SSL/TLS certificate for a Fly.io app.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"hostname": {
				Description: "The hostname to remove the certificate for (e.g. www.example.com)",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"certs", "remove", "--json"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			hostname, ok := args["hostname"]
			if !ok || hostname == "" {
				return nil, fmt.Errorf("missing required argument: hostname")
			}
			cmdArgs = append(cmdArgs, hostname)

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-certs-show",
		ToolDescription: "Show details for an SSL/TLS certificate for a Fly.io app.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"hostname": {
				Description: "The hostname to show the certificate for (e.g. www.example.com)",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"certs", "show", "--json"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			hostname, ok := args["hostname"]
			if !ok || hostname == "" {
				return nil, fmt.Errorf("missing required argument: hostname")
			}
			cmdArgs = append(cmdArgs, hostname)

			return cmdArgs, nil
		},
	},
}
