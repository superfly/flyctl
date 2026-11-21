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
		long = `Log in to Fly.io, log out, and check who you're authenticated as.

If you don't have an account, start with ` + "`fly auth signup`" + `; otherwise run
` + "`fly auth login`" + `. Check the current identity with ` + "`fly auth whoami`" + `.
In CI and other places without a browser, set ` + "`FLY_API_TOKEN`" + ` to a token
from ` + "`fly tokens create`" + ` instead of logging in.`
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

// Login runs the same flow as `fly auth login`. With no login flags on ctx it
// uses the browser flow.
func Login(ctx context.Context) error {
	return runLogin(ctx)
}

// Signup runs the same flow as `fly auth signup`.
func Signup(ctx context.Context) error {
	return runSignup(ctx)
}
