package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
)

func New() *cobra.Command {
	const (
		short = `Manage Managed Postgres clusters.`

		long = short + "\n"
	)

	cmd := command.New("mpg", short, long,
		func(ctx context.Context) error {
			// Check token compatibility early
			if err := validateMPGTokenCompatibility(ctx); err != nil {
				return err
			}
			return nil
		},
	)

	flag.Add(cmd,
		flag.Org(),
	)

	cmd.AddCommand(
		newProxy(),
		newConnect(),
		newAttach(),
		newDetach(),
		newStatus(),
		newList(),
		newCreate(),
		newDestroy(),
		newBackup(),
		newRestore(),
		newDatabases(),
		newUsers(),
	)

	return cmd
}

// detectTokenHasMacaroon determines if the current context has macaroon-style tokens.
// MPG commands require macaroon tokens to function properly.
func detectTokenHasMacaroon(ctx context.Context) bool {
	tokens := config.Tokens(ctx)
	if tokens == nil {
		return false
	}

	// Check for macaroon tokens (newer style)
	return len(tokens.GetMacaroonTokens()) > 0
}

// validateMPGTokenCompatibility checks if the current authentication tokens are compatible
// with MPG commands. MPG requires macaroon-style tokens and cannot work with older bearer tokens.
// Returns an error if bearer tokens are detected, suggesting the user upgrade their tokens.
func validateMPGTokenCompatibility(ctx context.Context) error {
	if !detectTokenHasMacaroon(ctx) {
		return fmt.Errorf(`MPG commands require updated tokens but found older-style tokens.

Please upgrade your authentication by running:
  flyctl auth logout
  flyctl auth login
`)
	}

	return nil
}
