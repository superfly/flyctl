package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newLogout() *cobra.Command {
	const (
		long = `Log the currently logged-in user out of the Fly platform.
To continue interacting with Fly, the user will need to log in again.
`
		short = "Logs out the currently logged in user"
	)

	cmd := command.New("logout", short, long, runLogout)

	flag.Add(cmd,
		flag.Bool{
			Name:        "force",
			Description: "Remove the saved access token even if it could not be revoked",
		},
	)

	return cmd
}

func runLogout(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	path := state.ConfigFile(ctx)

	// Log out the local access token that `fly auth login` manages, rather than an
	// externally supplied token that happened to take precedence for this run.
	revoked, revokeErr := revokeSavedToken(ctx, path)
	if revokeErr != nil && !flag.GetBool(ctx, "force") {
		return fmt.Errorf("failed to revoke saved token: %w\n"+
			"The token is still saved locally, so you can retry. "+
			"Run `fly auth logout --force` to remove it anyway and revoke it in the dashboard", revokeErr)
	}

	// Clear the saved token before doing anything else that can fail. A token that
	// has been revoked but left on disk wedges every future logout, because
	// revoking it again keeps failing.
	if err := config.Clear(path); err != nil {
		return fmt.Errorf("failed clearing config file at %s: %w", path, err)
	}

	if ac, err := agent.DefaultClient(ctx); err == nil {
		if err := ac.Kill(ctx); err != nil {
			// Not fatal: the saved token is already gone.
			warn(io, fmt.Sprintf("Failed stopping agent: %v", err))
		}
	}

	switch {
	case revokeErr != nil:
		fmt.Fprintln(io.Out, "removed local access token without revoking it")
		warn(io, fmt.Sprintf(
			"The saved access token could not be revoked (%v), so it remains valid. Revoke it in the dashboard.",
			revokeErr,
		))
	case revoked:
		fmt.Fprintln(io.Out, "logged out successfully")
	default:
		fmt.Fprintln(io.Out, "no local access token found")
	}

	if warning := logoutTokenOverrideWarning(); warning != "" {
		warn(io, warning)
	}

	return nil
}

func warn(io *iostreams.IOStreams, msg string) {
	colorize := io.ColorScheme()
	fmt.Fprintf(io.ErrOut, "\n%s %s\n", colorize.WarningIcon(), colorize.Yellow(msg))
}

func revokeSavedToken(ctx context.Context, path string) (bool, error) {
	token, err := config.ReadAccessToken(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading local access token: %w", err)
	}
	if token == "" {
		return false, nil
	}

	client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: token,
		BaseURL:     config.FromContext(ctx).APIBaseURL,
	})
	resp, err := gql.LogOut(ctx, client.GenqClient())
	if err != nil {
		return false, err
	}
	if !resp.LogOut.Ok {
		return false, errors.New("logout was not accepted by the API")
	}

	return true, nil
}

func logoutTokenOverrideWarning() string {
	warnFor := func(envVar string) string {
		return fmt.Sprintf(
			"Environment variable %s remains set. Its token was not revoked or removed, and flyctl will continue using it. Unset %s to stop using it; use `fly tokens revoke supplied` to revoke it.",
			envVar, envVar,
		)
	}

	// Keep this precedence in sync with config.applyEnv. An explicitly empty
	// FLY_ACCESS_TOKEN prevents FLY_API_TOKEN from being selected, so neither
	// token is in use and there is nothing to warn about.
	if token, ok := os.LookupEnv(config.AccessTokenEnvKey); ok {
		if token == "" {
			return ""
		}
		if apiToken := os.Getenv(config.APITokenEnvKey); apiToken != "" {
			return fmt.Sprintf(
				"Environment variables %s and %s remain set. Neither token was revoked or removed, and flyctl will continue using them. Unset both variables to stop using them or use `fly tokens revoke supplied` to revoke them, one at a time.",
				config.AccessTokenEnvKey, config.APITokenEnvKey,
			)
		}

		return warnFor(config.AccessTokenEnvKey)
	}

	if token := os.Getenv(config.APITokenEnvKey); token != "" {
		return warnFor(config.APITokenEnvKey)
	}

	return ""
}
