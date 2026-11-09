package tokens

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newRevokeSupplied() *cobra.Command {
	cmd := command.New("supplied", "Revoke an access token supplied by flag or environment", `Revoke the access token supplied by --access-token, FLY_ACCESS_TOKEN, or
FLY_API_TOKEN, including all tokens in a token bundle. Uses --access-token first,
then FLY_ACCESS_TOKEN, then FLY_API_TOKEN. An explicitly empty FLY_ACCESS_TOKEN
prevents FLY_API_TOKEN from being selected.

Use 'fly auth logout' to revoke the access token saved by 'fly auth login'.

Does not clear local configuration or unset environment variables.`, runRevokeSupplied)
	cmd.Args = cobra.NoArgs

	return cmd
}

func runRevokeSupplied(ctx context.Context) error {
	source := suppliedTokenSource(ctx)
	if source == "" {
		return fmt.Errorf("No access token supplied by --access-token, FLY_ACCESS_TOKEN, or FLY_API_TOKEN. Use `fly auth logout` to revoke your saved login token.")
	}
	client := flyutil.ClientFromContext(ctx)
	// Guard against --access-token being set to an empty string.
	if !client.Authenticated() {
		return fmt.Errorf("no access token selected")
	}

	resp, err := gql.LogOut(ctx, client.GenqClient())
	if err != nil {
		return fmt.Errorf("failed to revoke access token from %s: %w", source, err)
	}
	if !resp.LogOut.Ok {
		return fmt.Errorf("failed to revoke access token from %s: the API did not accept the revocation, try revoking the token in the dashboard", source)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Revoked access token(s) from %s.\n", source)
	var warning string
	switch source {
	case config.AccessTokenEnvKey:
		warning = fmt.Sprintf("Unset %s before running further commands.", source)
		if os.Getenv(config.APITokenEnvKey) != "" {
			warning += fmt.Sprintf(" %s was not targeted; flyctl will use it once %s is unset. Unset both variables to use your saved access token.", config.APITokenEnvKey, source)
		}
	case config.APITokenEnvKey:
		warning = fmt.Sprintf("Unset %s before running further commands.", source)
	case "--access-token":
		warning = "Stop passing this token with --access-token."
	}
	if warning != "" {
		colorize := io.ColorScheme()
		fmt.Fprintf(io.ErrOut, "\n%s %s\n", colorize.WarningIcon(), colorize.Yellow(warning))
	}

	return nil
}

func suppliedTokenSource(ctx context.Context) string {
	if flag.FromContext(ctx).Changed(flagnames.AccessToken) {
		return "--access-token"
	}
	// Match config.applyEnv's first-present-variable precedence, including empty values.
	if token, ok := os.LookupEnv(config.AccessTokenEnvKey); ok {
		if token != "" {
			return config.AccessTokenEnvKey
		}
	} else if os.Getenv(config.APITokenEnvKey) != "" {
		return config.APITokenEnvKey
	}

	return ""
}
