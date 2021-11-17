package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/env"
)

func newLogout() *cobra.Command {
	const (
		long = `Log the currently logged-in user out of the Fly platform. 
To continue interacting with Fly, the user will need to log in again.
`
		short = "Logs out the currently logged in user"
	)

	return command.New("logout", short, long, runLogout,
		command.RequireSession)
}

func runLogout(ctx context.Context) error {
	if err := agent.StopRunningAgent(); err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	path := state.ConfigFile(ctx)
	if err := config.Unset(path, config.AccessTokenFileKey); err != nil {
		// TODO: exit code depending on whether key removal took place

		fmt.Fprintf(io.ErrOut, "failed unsetting %s in %s: %v\n",
			config.AccessTokenFileKey, path, err)
	}

	keyExists := env.IsSet(config.APITokenEnvKey)
	tokenExists := env.IsSet(config.AccessTokenEnvKey)

	single := func(key string) {
		fmt.Fprintf(io.ErrOut, "$%s is set in your environment; don't forget to remove it.")
	}

	switch {
	case keyExists && tokenExists:
		const msg = "$%s & $%s are set in your environment; don't forget to remove them.\n"

		fmt.Fprintf(io.ErrOut, msg, config.APITokenEnvKey, config.AccessTokenEnvKey)
	case keyExists:
		single(config.APITokenEnvKey)
	case tokenExists:
		single(config.AccessTokenEnvKey)
	}

	return nil
}
