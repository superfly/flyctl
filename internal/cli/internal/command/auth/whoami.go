package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

func newWhoAmI() *cobra.Command {
	const (
		long = `Displays the users email address/service identity currently 
authenticated and in use.
`
		short = "Show the currently authenticated user"
	)

	return command.New("whoami", long, short, runWhoAmI,
		command.RequireSession)
}

func runWhoAmI(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Current user: %s\n", user.Email)

	return nil
}
