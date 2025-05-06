package mcpServer

var PlatformCommands = []FlyCommand{
	{
		ToolName:        "fly-platform-regions",
		ToolDescription: "Get a list of regions where Fly has edges and/or datacenters",
		ToolArgs:        map[string]FlyArg{},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"platform", "regions", "--json"}
			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-platform-status",
		ToolDescription: "Get the status of Fly's platform",
		ToolArgs:        map[string]FlyArg{},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"platform", "status", "--json"}
			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-platform-vm-sizes",
		ToolDescription: "Get a list of VM sizes available for Fly apps",
		ToolArgs:        map[string]FlyArg{},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"platform", "vm-sizes", "--json"}
			return cmdArgs, nil
		},
	},
}
