package mcp

var LogCommands = []FlyCommand{
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
}
