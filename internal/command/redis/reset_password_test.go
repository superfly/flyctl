package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// Without the provider the API finds the add-on by name alone, which can be
// another provider's add-on with the same name, such as a Tigris bucket.
func TestResetPasswordSendsProvider(t *testing.T) {
	client := &captureGenqClient{}

	_, err := resetPassword(context.Background(), client.client(), "cache")
	require.NoError(t, err)

	assert.Equal(t, 1, client.requests)
	assert.Equal(t, []string{"ResetAddOnPassword"}, client.opNames)
	assert.Equal(t, map[string]any{"name": "cache", "provider": "upstash_redis"}, client.variables)
}

// runResetWith runs `fly redis reset <args>` against a GraphQL client that
// answers with respond, and returns what the command printed.
func runResetWith(t *testing.T, respond mock.GraphQLClient, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	io, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GenqClientFunc: func() genq.Client { return respond },
	})
	flags := pflag.NewFlagSet("reset", pflag.ContinueOnError)
	require.NoError(t, flags.Parse(args))
	ctx = flagctx.NewContext(ctx, flags)

	err = runReset(ctx)

	return out.String(), errOut.String(), err
}

func TestRunResetPrintsNewURL(t *testing.T) {
	var variables map[string]any
	respond := mock.GraphQLClient(func(_ context.Context, req *genq.Request, resp *genq.Response) error {
		encoded, err := json.Marshal(req.Variables)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &variables); err != nil {
			return err
		}

		return json.Unmarshal([]byte(`{"resetAddOnPassword":{"addOn":{"publicUrl":"redis://default:new-password@fly-cache.upstash.io:6379"}}}`), resp.Data)
	})

	stdout, stderr, err := runResetWith(t, respond, "cache")
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"name": "cache", "provider": "upstash_redis"}, variables)
	assert.Contains(t, stdout, "The password for your Redis database cache was reset.")
	assert.Contains(t, stdout, "redis://default:new-password@fly-cache.upstash.io:6379")
	assert.Empty(t, stderr)
}

// A failed reset, such as no Upstash Redis with that name, must return the
// API's error and must not claim the password was reset.
func TestRunResetReturnsAPIError(t *testing.T) {
	respond := mock.GraphQLClient(func(context.Context, *genq.Request, *genq.Response) error {
		return errors.New("Could not find AddOn")
	})

	stdout, _, err := runResetWith(t, respond, "missing")
	require.ErrorContains(t, err, "Could not find AddOn")

	assert.NotContains(t, stdout, "was reset")
	assert.NotContains(t, stdout, "redis://")
}

func TestResetRequiresExactlyOneName(t *testing.T) {
	cmd := newReset()

	assert.Error(t, cmd.Args(cmd, nil))
	assert.Error(t, cmd.Args(cmd, []string{"one", "two"}))
	assert.NoError(t, cmd.Args(cmd, []string{"cache"}))
}
