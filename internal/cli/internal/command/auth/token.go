package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
)

func newToken() *cobra.Command {
	const (
		long = `Shows the authentication token that is currently in use. 
This can be used as an authentication token with API services, 
independent of flyctl.
`
		short = "Show the current auth token"
	)

	return command.New("token", short, long, runAuthToken,
		command.RequireSession)
}

func runAuthToken(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	token := cfg.AccessToken()

	if io := iostreams.FromContext(ctx); cfg.JSONOutput() {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}
