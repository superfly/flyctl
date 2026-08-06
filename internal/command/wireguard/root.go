package wireguard

import (
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Commands that manage WireGuard peer connections"
		long  = `Commands that manage WireGuard peer connections`
	)
	cmd := command.New("wireguard", short, long, nil)
	cmd.Aliases = []string{"wg"}
	cmd.AddCommand(
		newWireguardList(),
		newWireguardCreate(),
		newWireguardRemove(),
		newWireguardReset(),
		newWireguardWebsockets(),
	)

	return cmd
}

func newWireguardList() *cobra.Command {
	const (
		short = "List all WireGuard peer connections"
		long  = `List all WireGuard peer connections`
	)
	cmd := command.New("list [org]", short, long, runWireguardList,
		command.RequireSession,
	)
	flag.Add(cmd,
		flag.JSONOutput(),
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func newWireguardCreate() *cobra.Command {
	const (
		short = "Add a WireGuard peer connection"
		long  = `Add a WireGuard peer connection to an organization`
	)
	cmd := command.New("create [org] [region] [name] [file]", short, long, runWireguardCreate,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(4)
	flag.Add(cmd,
		flag.String{
			Name:        "network",
			Description: "Custom network name",
		},
	)

	return cmd
}

func newWireguardRemove() *cobra.Command {
	const (
		short = "Remove a WireGuard peer connection"
		long  = `Remove a WireGuard peer connection from an organization`
	)
	cmd := command.New("remove [org] [name]", short, long, runWireguardRemove,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func newWireguardReset() *cobra.Command {
	const (
		short = "Reset WireGuard peer connection for an organization"
		long  = `Reset WireGuard peer connection for an organization`
	)
	cmd := command.New("reset [org]", short, long, runWireguardReset,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func newWireguardWebsockets() *cobra.Command {
	const (
		short = "Enable or disable WireGuard tunneling over WebSockets"
		long  = `Enable or disable WireGuard tunneling over WebSockets`
	)
	cmd := command.New("websockets [enable|disable]", short, long, runWireguardWebsockets,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}
