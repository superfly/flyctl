package mcpServer

import (
	"fmt"
	"strings"

	"github.com/google/shlex"
)

var SecretsCommands = []FlyCommand{
	{
		ToolName:        "fly-secrets-deploy",
		ToolDescription: "Deploy secrets to the specified app",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"secrets", "deploy"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-secrets-list",
		ToolDescription: "List secrets for the specified app",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"secrets", "list", "--json"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-secrets-set",
		ToolDescription: "Set secrets for the specified app; secrets are staged for the next deploy",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"keyvalues": {
				Description: "Secrets to set in KEY=VALUE format",
				Required:    true,
				Type:        "hash",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"secrets", "set", "--stage"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			keyvalues, ok := args["keyvalues"]
			if ok && keyvalues != "" {
				args, err := shlex.Split(keyvalues)
				if err != nil {
					return nil, fmt.Errorf("failed to parse keyvalues: %w", err)
				}
				cmdArgs = append(cmdArgs, args...)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-secrets-unset",
		ToolDescription: "Unset secrets for the specified app",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"names": {
				Description: "Names of secrets to unset",
				Required:    true,
				Type:        "array",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"secrets", "unset"}

			app, ok := args["app"]
			if !ok || app == "" {
				return nil, fmt.Errorf("missing required argument: app")
			}
			cmdArgs = append(cmdArgs, "-a", app)

			names, ok := args["names"]
			if ok && names != "" {
				cmdArgs = append(cmdArgs, strings.Split(names, ",")...)
			}

			return cmdArgs, nil
		},
	},
}
