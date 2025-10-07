package mcpServer

import (
	"fmt"
	"strconv"
	"strings"
)

var IPCommands = []FlyCommand{
	{
		ToolName:        "fly-ips-allocate-v4",
		ToolDescription: "Allocate an IPv4 address to the application. Dedicated IPv4 addresses cost $2/mo.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"shared": {
				Description: "Allocate a shared IPv4 address instead of dedicated",
				Required:    false,
				Type:        "boolean",
				Default:     "false",
			},
			"region": {
				Description: "Region to allocate the IP address in",
				Required:    false,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"ips", "allocate-v4"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "--app", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if shared, ok := args["shared"]; ok {
				if value, err := strconv.ParseBool(shared); err == nil && value {
					cmdArgs = append(cmdArgs, "--shared")
				}
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			// Always use --yes to avoid interactive prompts
			cmdArgs = append(cmdArgs, "--yes")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-ips-allocate-v6",
		ToolDescription: "Allocate an IPv6 address to the application",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"private": {
				Description: "Allocate a private IPv6 address",
				Required:    false,
				Type:        "boolean",
				Default:     "false",
			},
			"region": {
				Description: "Region to allocate the IP address in",
				Required:    false,
				Type:        "string",
			},
			"org": {
				Description: "Organization slug (required for private IPv6)",
				Required:    false,
				Type:        "string",
			},
			"network": {
				Description: "Target network name for a Flycast private IPv6 address",
				Required:    false,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"ips", "allocate-v6"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "--app", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if private, ok := args["private"]; ok {
				if value, err := strconv.ParseBool(private); err == nil && value {
					cmdArgs = append(cmdArgs, "--private")
				}
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			if org, ok := args["org"]; ok {
				cmdArgs = append(cmdArgs, "--org", org)
			}

			if network, ok := args["network"]; ok {
				cmdArgs = append(cmdArgs, "--network", network)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-ips-list",
		ToolDescription: "List all IP addresses allocated to the application",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"ips", "list"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "--app", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-ips-release",
		ToolDescription: "Release one or more IP addresses from the application",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"addresses": {
				Description: "IP addresses to release",
				Required:    true,
				Type:        "array",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"ips", "release"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "--app", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if addresses, ok := args["addresses"]; ok {
				// Split comma-separated addresses and add each as a separate argument
				for _, addr := range strings.Split(addresses, ",") {
					addr = strings.TrimSpace(addr)
					if addr != "" {
						cmdArgs = append(cmdArgs, addr)
					}
				}
			} else {
				return nil, fmt.Errorf("missing required argument: addresses")
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-ips-private",
		ToolDescription: "List instances' private IP addresses, accessible from within the Fly network",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"ips", "private"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "--app", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},
}
