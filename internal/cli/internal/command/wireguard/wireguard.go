// Package wireguard implements the wireguard command chain.
package wireguard

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		short = "Commands that manage WireGuard peer connections"
		long  = short + "n"
		usage = "wireguard <command>"
	)

	// TODO: list should also accept the --org param

	wg := command.New(usage, short, long, nil)

	wg.AddCommand(
		newList(),
	)

	return wg
}
