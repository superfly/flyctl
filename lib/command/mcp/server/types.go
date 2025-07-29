package mcpServer

// This file defines the structure and types used for Fly commands in the MCP server.
// As JSON-RPC schema wrapped in MCP go functions is a bit verbose, we define a simpler
// structure here to make it easier to define and dispatch commands.  This contains only
// the things needed to build the command line arguments for the flyctl CLI.

// mcp.runServer defines each tool based on the definition found in the FlyCommand struct.

// The tool function is responsible for converting the arguments into a slice of strings
// that can be passed to the Builder.  This function should return an error if the arguments
// are invalid or if there is an issue building the command line arguments.

// Argument values passed to the Builder are intended to be passed to exec.Command, and therefore
// are strings. The builder is responsible for constructing a flyctl command from the arguments,
// expressed as a slice of strings.  The builder should return an error if there is an issue
// building the command line arguments, or if the arguments are invalid.

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
	Type        string // "string", "enum", "array", "number", "boolean"
	Default     string
	Enum        []string
}
