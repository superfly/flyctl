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
		long = `DEPRECATED: Shows the authentication token that is currently
in use by flyctl. The auth token used by flyctl may expire quickly and
shouldn't be used in places where the token needs to keep working for a long
time. For API authentication, you can use the "fly tokens create" command
instead, to create narrowly-scoped tokens with a custom expiry.`

		short = "Show the current auth token in use by flyctl."
	)

	cmd := command.New("token", short, long, runAuthToken,
		command.ExcludeFromMetrics,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Suppress deprecation warning",
		},
	)

	cmd.Hidden = true

	return cmd
}

func runAuthToken(ctx context.Context) error {
	var (
		cfg   = config.FromContext(ctx)
		token = cfg.Tokens.GraphQL()
		io    = iostreams.FromContext(ctx)
	)

	if cfg.JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
		return nil
	}

	if !flag.GetBool(ctx, "quiet") {
		fmt.Fprintln(io.ErrOut, io.ColorScheme().Yellow("The 'fly auth token' command is deprecated. Use 'fly tokens create' instead."))
	}

	fmt.Fprintln(io.Out, token)
	return nil
}
