package mcp

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = `flyctl Model Context Protocol.`

		long = short + "\n"
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
	)

	return cmd
}
