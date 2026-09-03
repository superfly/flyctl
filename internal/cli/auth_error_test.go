package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/superfly/graphql"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

func TestWithAuthTokenOverrideSuggestion(t *testing.T) {
	unauthorized := &graphql.GraphQLError{
		Message: "You must be authenticated to view this.",
		Extensions: graphql.GraphQLErrorExtensions{
			Code: "UNAUTHORIZED",
		},
	}

	t.Run("FLY_API_TOKEN", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "restored-after-test")
		require.NoError(t, os.Unsetenv(config.AccessTokenEnvKey))
		t.Setenv(config.APITokenEnvKey, "bad-token")

		err := withAuthTokenOverrideSuggestion(nil, unauthorized)
		suggestion := flyerr.GetErrorSuggestion(err)

		require.ErrorIs(t, err, unauthorized)
		require.Contains(t, suggestion, config.APITokenEnvKey)
		require.Contains(t, suggestion, "overrides credentials saved by `fly auth login`")
	})

	t.Run("FLY_ACCESS_TOKEN", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "bad-token")
		t.Setenv(config.APITokenEnvKey, "another-token")

		err := withAuthTokenOverrideSuggestion(nil, unauthorized)
		suggestion := flyerr.GetErrorSuggestion(err)

		require.ErrorIs(t, err, unauthorized)
		require.Contains(t, suggestion, config.AccessTokenEnvKey)
		require.NotContains(t, suggestion, config.APITokenEnvKey)
		require.Contains(t, suggestion, "overrides credentials saved by `fly auth login`")
	})

	t.Run("access token flag takes precedence", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "env-token")
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String(flagnames.AccessToken, "", "")
		require.NoError(t, cmd.Flags().Set(flagnames.AccessToken, "flag-token"))

		err := withAuthTokenOverrideSuggestion(cmd, unauthorized)
		suggestion := flyerr.GetErrorSuggestion(err)

		require.ErrorIs(t, err, unauthorized)
		require.Contains(t, suggestion, "--access-token")
		require.NotContains(t, suggestion, config.AccessTokenEnvKey)
	})

	t.Run("wrapped unauthorized error", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "bad-token")

		err := withAuthTokenOverrideSuggestion(nil, fmt.Errorf("query failed: %w", unauthorized))
		suggestion := flyerr.GetErrorSuggestion(err)

		require.ErrorIs(t, err, unauthorized)
		require.Contains(t, suggestion, config.AccessTokenEnvKey)
	})

	t.Run("other GraphQL error", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "token")
		err := &graphql.GraphQLError{
			Message: "forbidden",
			Extensions: graphql.GraphQLErrorExtensions{
				Code: "FORBIDDEN",
			},
		}

		require.Same(t, err, withAuthTokenOverrideSuggestion(nil, err))
	})

	t.Run("no override", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "")
		t.Setenv(config.APITokenEnvKey, "")

		require.Same(t, unauthorized, withAuthTokenOverrideSuggestion(nil, unauthorized))
	})

	t.Run("empty FLY_ACCESS_TOKEN blocks FLY_API_TOKEN", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "")
		t.Setenv(config.APITokenEnvKey, "bad-token")

		require.Same(t, unauthorized, withAuthTokenOverrideSuggestion(nil, unauthorized))
	})

	t.Run("preserves existing suggestion", func(t *testing.T) {
		t.Setenv(config.AccessTokenEnvKey, "bad-token")
		err := flyerr.WithSuggestion(unauthorized, "existing suggestion")

		err = withAuthTokenOverrideSuggestion(nil, err)

		require.ErrorIs(t, err, unauthorized)
		require.Contains(t, flyerr.GetErrorSuggestion(err), config.AccessTokenEnvKey)
		require.Contains(t, flyerr.GetErrorSuggestion(err), "existing suggestion")
	})
}

func TestPrintErrorIncludesAuthTokenOverrideSuggestion(t *testing.T) {
	t.Setenv(config.AccessTokenEnvKey, "restored-after-test")
	require.NoError(t, os.Unsetenv(config.AccessTokenEnvKey))
	t.Setenv(config.APITokenEnvKey, "bad-token")
	var err error = &graphql.GraphQLError{
		Message: "You must be authenticated to view this.",
		Extensions: graphql.GraphQLErrorExtensions{
			Code: "UNAUTHORIZED",
		},
	}
	io, _, _, stderr := iostreams.Test()
	err = withAuthTokenOverrideSuggestion(nil, err)

	printError(io, io.ColorScheme(), nil, err)

	require.Equal(t, "Error: You must be authenticated to view this.\n"+
		"The token provided by FLY_API_TOKEN was rejected for this request. "+
		"This environment variable overrides credentials saved by `fly auth login`; "+
		"check its value and permissions, or unset it and try again.\n\n", stderr.String())
}
