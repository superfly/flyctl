package wireguard

import (
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Manage WireGuard peer connections"
		long  = `Manage the WireGuard peers that connect computers to an organization's
private network. Use ` + "`fly wireguard create`" + ` to get a configuration for
your own WireGuard client, and ` + "`fly wireguard list`" + ` and
` + "`fly wireguard remove`" + ` to clean up. Commands such as ` + "`fly ssh`" + ` and
` + "`fly proxy`" + ` set up their own peers through the Fly agent; see
` + "`fly agent`" + `.`
	)
	cmd := command.New("wireguard", short, long, nil)
	cmd.Aliases = []string{"wg"}
	cmd.AddCommand(
		newWireguardList(),
		newWireguardCreate(),
		newWireguardRemove(),
		newWireguardReset(),
		newWireguardWebsockets(),
		newWireguardTokenMode(),
	)

	return cmd
}

func newWireguardList() *cobra.Command {
	const (
		short = "List all WireGuard peer connections"
		long  = `List the WireGuard peers of an organization with their name, region and
peer IP. Pass the organization slug as the argument; without it you pick
one from a list. Use ` + "`--json`" + ` for machine-readable output.`
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
		long  = `Create a WireGuard peer in an organization and write a configuration file
for a standard WireGuard client. Arguments are positional: organization,
region, peer name and output file. Without an organization you pick one
from a list; region and name are optional.

The output file must not exist yet; pass "stdout" to print the configuration
instead. Without a file argument you are prompted for one, and without a
terminal the file argument is required. The configuration contains the
peer's private key, which cannot be recovered later; if you lose it, remove
the peer and create a new one.`
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
		long  = `Delete a WireGuard peer from an organization. Pass the organization and
peer name as arguments; without them you pick from lists, and without a
terminal both are required. See peer names with ` + "`fly wireguard list`" + `.`
	)
	cmd := command.New("remove [org] [name]", short, long, runWireguardRemove,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func newWireguardReset() *cobra.Command {
	const (
		short = "Reset the Fly agent's WireGuard peer for an organization"
		long  = `Have the Fly agent replace the WireGuard peer it uses for an organization
with a new one, and print the new peer's name. Try this when ` + "`fly ssh`" + `
or ` + "`fly proxy`" + ` can't connect to that organization. Pass the organization
slug as the argument; without it you pick one from a list. Starts the agent
if it isn't running.`
	)
	cmd := command.New("reset [org]", short, long, runWireguardReset,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func newWireguardTokenMode() *cobra.Command {
	const (
		short = "Enable or disable token-mode WireGuard peers"
		long  = `Enable or disable token-mode WireGuard peers.

In token mode the agent provisions WireGuard peers inline at the token
gateway, authenticating with the session's tokens, instead of registering
them through the API. This setting is the local default; the agent also
consults the wireguard-token-mode feature flag each time it builds a tunnel,
and the flag can turn token mode on for a user whose local setting is off.

The setting is saved in your flyctl config file, and the running agent is
stopped so the next command that needs it starts one with the new setting.
`
	)
	cmd := command.New("token-mode [enable|disable]", short, long, runWireguardTokenMode,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newWireguardWebsockets() *cobra.Command {
	const (
		short = "Enable or disable WireGuard tunneling over WebSockets"
		long  = `Turn WireGuard tunneling over WebSockets on or off for the Fly agent. Pass
"enable" or "disable".

The setting is saved in your flyctl config file, and the running agent is
stopped so the next command that needs it starts one with the new setting.`
	)
	cmd := command.New("websockets [enable|disable]", short, long, runWireguardWebsockets,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}
