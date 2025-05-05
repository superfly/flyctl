package mcp

// FlyCommand represents a command for the Fly MCP server
type FlyCommand struct {
	ToolName        string
	ToolDescription string
	ToolArgs        map[string]FlyArg
	Builder         func(args map[string]string) ([]string, error)
}

// FlyArg represents an argument for a Fly command
type FlyArg struct {
	Description string
	Required    bool
	Type        string
}
