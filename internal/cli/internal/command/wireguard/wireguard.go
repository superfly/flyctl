// Package wireguard implements the wireguard command chain.
package wireguard

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/wireguard/token"
)

func New() *cobra.Command {
	const (
		short = "Commands that manage WireGuard peer connections"
		long  = short + "\n"
		usage = "wireguard <command>"
	)

	// TODO: list should also accept the --org param

	wg := command.New(usage, short, long, nil)

	wg.AddCommand(
		token.New(),
		newList(),
	)

	return wg
}
