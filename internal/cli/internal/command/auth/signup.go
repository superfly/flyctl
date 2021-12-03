package auth

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newSignup() *cobra.Command {
	const (
		long = `Creates a new fly account. The command opens the browser 
and sends the user to a form to provide appropriate credentials.
`
		short = "Create a new fly account"
	)

	return command.New("signup", short, long, runSignup)
}

func runSignup(ctx context.Context) error {
	return runWebLogin(ctx, true)
}
