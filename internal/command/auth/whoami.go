package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
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
	cfg := config.FromContext(ctx)

	if cfg.JSONOutput {
		_ = render.JSON(io.Out, map[string]string{"email": user.Email})
	} else {
		fmt.Fprintln(io.Out, user.Email)
	}

	return nil
}
