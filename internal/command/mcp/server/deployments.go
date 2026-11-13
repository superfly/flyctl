package mcpServer

import "fmt"

// Deployments of apps that deploy from GitHub through the Fly.io dashboard
// (`fly deployments`). Launching isn't exposed: it hands steps to a person
// (installing the GitHub App, adding a payment method) better done from the CLI.

var appArg = FlyArg{
	Description: "Name of the app",
	Required:    true,
	Type:        "string",
}

var deploymentIDArg = FlyArg{
	Description: "Deployment ID, or latest",
	Required:    false,
	Type:        "string",
	Default:     "latest",
}

func deploymentCommand(sub string, json bool) func(args map[string]string) ([]string, error) {
	return func(args map[string]string) ([]string, error) {
		cmdArgs := []string{"deployments", sub}

		if id, ok := args["id"]; ok && id != "" {
			cmdArgs = append(cmdArgs, id)
		}
		if app, ok := args["app"]; ok {
			cmdArgs = append(cmdArgs, "-a", app)
		}
		if json {
			cmdArgs = append(cmdArgs, "--json")
		}

		return cmdArgs, nil
	}
}

var DeploymentCommands = []FlyCommand{
	{
		ToolName:        "fly-deployments-list",
		ToolDescription: "List deployments of a Fly.io app that deploys from GitHub, newest first",
		ToolArgs: map[string]FlyArg{
			"app": appArg,
			"limit": {
				Description: "Number of deployments to list (max 100)",
				Required:    false,
				Type:        "number",
				Default:     "20",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"deployments", "list", "--json", "-a", args["app"]}

			if limit, ok := args["limit"]; ok {
				cmdArgs = append(cmdArgs, "--limit", limit)
			}

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-deployments-show",
		ToolDescription: "Show a deployment of a Fly.io app that deploys from GitHub: status, steps, errors and repository settings",
		ToolArgs:        map[string]FlyArg{"app": appArg, "id": deploymentIDArg},
		Builder:         deploymentCommand("show", true),
	},
	{
		ToolName: "fly-deployments-diagnose",
		ToolDescription: "Explain why a deployment from GitHub failed and how to fix the repository: " +
			"failed step, errors, log excerpt, repository settings, last good commit and next steps (Markdown). " +
			"Start here when a deploy failed.",
		ToolArgs: map[string]FlyArg{
			"app": appArg,
			"id":  deploymentIDArg,
			"lines": {
				Description: "Log lines to include (0-500)",
				Required:    false,
				Type:        "number",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs, _ := deploymentCommand("diagnose", false)(args)

			if lines, ok := args["lines"]; ok {
				cmdArgs = append(cmdArgs, "--lines", lines)
			}

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-deployments-logs",
		ToolDescription: "Get the log of a deployment from GitHub",
		ToolArgs: map[string]FlyArg{
			"app": appArg,
			"id":  deploymentIDArg,
			"step": {
				Description: "Only lines from this step, such as build",
				Required:    false,
				Type:        "string",
			},
			"limit": {
				Description: "Keep the last N lines",
				Required:    false,
				Type:        "number",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs, _ := deploymentCommand("logs", true)(args)

			if step, ok := args["step"]; ok {
				cmdArgs = append(cmdArgs, "--step", step)
			}
			if limit, ok := args["limit"]; ok {
				cmdArgs = append(cmdArgs, "--limit", limit)
			}

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-deployments-redeploy",
		ToolDescription: "Deploy a Fly.io app's GitHub repository (the settings' branch, or a given branch or commit)",
		ToolArgs: map[string]FlyArg{
			"app": appArg,
			"ref": {
				Description: "Branch or full commit SHA to deploy",
				Required:    false,
				Type:        "string",
			},
			"replace": {
				Description: "Stop a running deployment and start this one",
				Required:    false,
				Type:        "boolean",
				Default:     "false",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"deployments", "redeploy", "--json", "-a", args["app"]}

			if ref, ok := args["ref"]; ok && ref != "" {
				cmdArgs = append(cmdArgs, "--ref", ref)
			}
			if replace, ok := args["replace"]; ok {
				switch replace {
				case "true":
					cmdArgs = append(cmdArgs, "--replace")
				case "false":
				default:
					return nil, fmt.Errorf("invalid value for replace: %v", replace)
				}
			}

			return cmdArgs, nil
		},
	},
	{
		ToolName:        "fly-deployments-retry",
		ToolDescription: "Deploy the commit a finished deployment from GitHub built again",
		ToolArgs:        map[string]FlyArg{"app": appArg, "id": deploymentIDArg},
		Builder:         deploymentCommand("retry", true),
	},
	{
		ToolName:        "fly-deployments-cancel",
		ToolDescription: "Stop a running deployment from GitHub",
		ToolArgs:        map[string]FlyArg{"app": appArg, "id": deploymentIDArg},
		Builder:         deploymentCommand("cancel", true),
	},
}
