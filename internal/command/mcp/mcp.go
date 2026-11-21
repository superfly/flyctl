package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = `Run, deploy and connect Model Context Protocol (MCP) servers`

		long = `Experimental commands for Model Context Protocol (MCP) servers, which AI
clients such as Claude, Cursor and VS Code use to call tools.

` + "`fly mcp server`" + ` runs flyctl itself as an MCP server. ` + "`fly mcp launch`" + `
deploys an stdio MCP program as a Fly app, and ` + "`fly mcp add`" + `,
` + "`fly mcp remove`" + ` and ` + "`fly mcp list`" + ` manage the entries in local MCP
client config files. To connect coding agents to the hosted Fly MCP server
instead, use ` + "`fly setup agent`" + `.
`
	)

	cmd := command.New("mcp", short, long, nil)
	// cmd.Hidden = true

	cmd.AddCommand(
		NewProxy(),
		NewInspect(),
		newServer(),
		NewWrap(),

		NewAdd(),
		NewRemove(),

		NewLaunch(),
		NewDestroy(),

		newVolume(),
		newList(),
		newLogs(),
	)

	return cmd
}

func flyctl(args ...string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	log.Debugf("Running:", executable, strings.Join(args, " "))

	cmd := exec.Command(executable, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
