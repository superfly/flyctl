package mcpServer

var StatusCommands = []FlyCommand{
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
