// Package auth implements the auth command chain.
package auth

import (
	"github.com/superfly/flyctl/internal/cli/internal/command"

	"github.com/spf13/cobra"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long = `Authenticate with Fly (and logout if you need to).
If you do not have an account, start with the AUTH SIGNUP command.
If you do have and account, begin with the AUTH LOGIN subcommand.
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
