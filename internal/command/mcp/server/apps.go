package mcpServer

import (
	"fmt"
	"strconv"
)

var AppCommands = []FlyCommand{
	{
		ToolName:        "fly-apps-create",
		ToolDescription: "Create a new Fly.io app.  If you don't specify a name, one will be generated for you.",
		ToolArgs: map[string]FlyArg{
			"name": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"org": {
				Description: "Slug of the organization to create the app in",
				Required:    true,
				Type:        "string",
			},
			"network": {
				Description: "Custom network id",
				Required:    false,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"apps", "create"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				cmdArgs = append(cmdArgs, "--generate-name")
			}

			if org, ok := args["org"]; ok {
				cmdArgs = append(cmdArgs, "--org", org)
			}

			if network, ok := args["network"]; ok {
				cmdArgs = append(cmdArgs, "--network", network)
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-apps-destroy",
		ToolDescription: "Destroy a Fly.io app.  All machines and volumes will be destroyed.",
		ToolArgs: map[string]FlyArg{
			"name": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"apps", "destroy"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				return nil, fmt.Errorf("missing required argument: name")
			}

			cmdArgs = append(cmdArgs, "--yes")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-apps-list",
		ToolDescription: "List all Fly.io apps in the organization",
		ToolArgs: map[string]FlyArg{
			"org": {
				Description: "Slug of the organization to list apps for",
				Required:    false,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"apps", "list"}

			if org, ok := args["org"]; ok {
				cmdArgs = append(cmdArgs, "--org", org)
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-apps-move",
		ToolDescription: "Move a Fly.io app to a different organization",
		ToolArgs: map[string]FlyArg{
			"name": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"org": {
				Description: "Slug of the organization to move the app to",
				Required:    true,
				Type:        "string",
			},
			"skip-health-checks": {
				Description: "Skip health checks during the move",
				Required:    false,
				Type:        "bool",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"apps", "move"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				return nil, fmt.Errorf("missing required argument: name")
			}

			if org, ok := args["org"]; ok {
				cmdArgs = append(cmdArgs, "--org", org)
			} else {
				return nil, fmt.Errorf("missing required argument: org")
			}

			if skipHealthChecks, ok := args["skip-health-checks"]; ok {
				if value, err := strconv.ParseBool(skipHealthChecks); err == nil && value {
					cmdArgs = append(cmdArgs, "--skip-health-checks")
				}
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-apps-releases",
		ToolDescription: "List all releases for a Fly.io app, including type, when, success/fail and which user triggered the release.",
		ToolArgs: map[string]FlyArg{
			"name": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"apps", "releases"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				return nil, fmt.Errorf("missing required argument: name")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-apps-restart",
		ToolDescription: "Restart a Fly.io app. Perform a rolling restart against all running Machines.",
		ToolArgs: map[string]FlyArg{
			"name": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"force-stop": {
				Description: "Force stop the app before restarting",
				Required:    false,
				Type:        "bool",
			},
			"skip-health-checks": {
				Description: "Skip health checks during the restart",
				Required:    false,
				Type:        "bool",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"apps", "restart"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				return nil, fmt.Errorf("missing required argument: name")
			}

			if forceStop, ok := args["force-stop"]; ok {
				if value, err := strconv.ParseBool(forceStop); err == nil && value {
					cmdArgs = append(cmdArgs, "--force-stop")
				}
			}

			if skipHealthChecks, ok := args["skip-health-checks"]; ok {
				if value, err := strconv.ParseBool(skipHealthChecks); err == nil && value {
					cmdArgs = append(cmdArgs, "--skip-health-checks")
				}
			}

			return cmdArgs, nil
		},
	},
}
