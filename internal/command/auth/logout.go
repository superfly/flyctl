package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/state"
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

func runLogout(ctx context.Context) (err error) {
	var ac *agent.Client
	if ac, err = agent.DefaultClient(ctx); err == nil {
		if err = ac.Kill(ctx); err != nil {
			err = fmt.Errorf("failed stopping agent: %w", err)

			return
		}
	}

	path := state.ConfigFile(ctx)
	if err = config.Clear(path); err != nil {
		err = fmt.Errorf("failed clearing config file at %s: %w\n", path, err)

		return
	}

	out := iostreams.FromContext(ctx).ErrOut

	single := func(key string) {
		fmt.Fprintf(out,
			"$%s is set in your environment; don't forget to remove it.", key)
	}

	keyExists := env.IsSet(config.APITokenEnvKey)
	tokenExists := env.IsSet(config.AccessTokenEnvKey)

	switch {
	case keyExists && tokenExists:
		const msg = "$%s & $%s are set in your environment; don't forget to remove them.\n"

		fmt.Fprintf(out, msg, config.APITokenEnvKey, config.AccessTokenEnvKey)
	case keyExists:
		single(config.APITokenEnvKey)
	case tokenExists:
		single(config.AccessTokenEnvKey)
	}

	return
}
