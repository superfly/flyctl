package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newToken() *cobra.Command {
	const (
		long = `Shows the authentication token that is currently in use by flyctl.
The auth token used by flyctl may expire quickly and shouldn't be used in places
where the token needs to keep working for a long time. For API authentication, you
can use the fly tokens create command instead, to create narrowly-scoped tokens with
a custom expiry.`

		short = "Show the current auth token in use by flyctl."
	)

	cmd := command.New("token", short, long, runAuthToken,
		command.ExcludeFromMetrics,
		command.RequireSession,
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runAuthToken(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	token := cfg.Tokens.GraphQL()

	if io := iostreams.FromContext(ctx); cfg.JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}
