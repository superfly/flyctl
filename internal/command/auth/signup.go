package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/iostreams"
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
	token, err := runWebLogin(ctx, true)
	if err != nil {
		return err
	}

	if err := persistAccessToken(ctx, token); err != nil {
		return err
	}

	user, err := client.FromToken(token).API().GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "successfully logged in as %s\n", colorize.Bold(user.Email))

	return nil
}
