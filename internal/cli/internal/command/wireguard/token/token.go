// Package token implements the wireguard token command chain.
package token

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Commands that manage WireGuard delegated access tokens"
		long  = short + "n"
		usage = "token <command>"
	)

	// TODO: list should also accept the --org param

	cmd = command.New(usage, short, long, nil)

	cmd.AddCommand(
		newList(),
		newCreate(),
		newDelete(),
	)

	return
}

func nameFromFirstArgOrPrompt(ctx context.Context) (name string, err error) {
	if name = flag.FirstArg(ctx); name == "" {
		err = prompt.String(ctx, &name, "Enter WireGuard token name:", "", true)
	}

	return
}
