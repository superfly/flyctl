// Package auth implements the auth command chain.
package auth

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long = `Authenticate with Fly (and logout if you need to).
If you do not have an account, start with the AUTH SIGNUP command.
If you do have an account, begin with the AUTH LOGIN subcommand.
`
		short = "Manage authentication"
	)

	auth := command.New("auth", short, long, nil)

	auth.AddCommand(
		newWhoAmI(),
		newToken(),
		newLogin(),
		newDocker(),
		newLogout(),
		newSignup(),
	)

	return auth
}

// RunLogin runs the same flow as `fly auth login` with no extra flags.
func RunLogin(ctx context.Context) error {
	return runLogin(ctx)
}

// RunSignup runs the same flow as `fly auth signup`.
func RunSignup(ctx context.Context) error {
	return runSignup(ctx)
}
