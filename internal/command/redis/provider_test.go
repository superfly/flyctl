package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// The redis commands that look a database up by name all send this provider,
// so the lookup can't land on another kind of add-on with the same name.
// proxy and connect send it too, after a prompt these tests can't answer.
func TestRedisProviderIsUpstashRedis(t *testing.T) {
	assert.Equal(t, "upstash_redis", redisProvider)
}

type recordedRequest struct {
	op        string
	variables map[string]any
}

// recordingAPI records each request and answers it with the JSON in replies,
// keyed by operation name. An operation with no reply fails with errNoReply.
type recordingAPI struct {
	replies  map[string]string
	requests []recordedRequest
}

var errNoReply = errors.New("no reply for this operation")

func (a *recordingAPI) client() genq.Client {
	return mock.GraphQLClient(func(_ context.Context, req *genq.Request, resp *genq.Response) error {
		var variables map[string]any
		encoded, err := json.Marshal(req.Variables)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &variables); err != nil {
			return err
		}
		a.requests = append(a.requests, recordedRequest{op: req.OpName, variables: variables})

		reply, ok := a.replies[req.OpName]
		if !ok {
			return errNoReply
		}

		return json.Unmarshal([]byte(reply), resp.Data)
	})
}

// runCommand runs cmd's run function with args parsed by its own flags,
// against api, and returns what it printed. Stdin is not a terminal, so any
// prompt fails as non-interactive.
func runCommand(t *testing.T, cmd *cobra.Command, run func(context.Context) error, api *recordingAPI, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	io, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GenqClientFunc: func() genq.Client { return api.client() },
	})
	flags := cmd.Flags()
	require.NoError(t, flags.Parse(args))
	ctx = flagctx.NewContext(ctx, flags)

	err = run(ctx)

	return out.String(), errOut.String(), err
}

func TestDestroySendsProvider(t *testing.T) {
	api := &recordingAPI{replies: map[string]string{"DeleteAddOn": `{}`}}

	stdout, _, err := runCommand(t, newDestroy(), runDestroy, api, "--yes", "cache")
	require.NoError(t, err)

	require.Len(t, api.requests, 1)
	assert.Equal(t, "DeleteAddOn", api.requests[0].op)
	assert.Equal(t, "cache", api.requests[0].variables["name"])
	assert.Equal(t, "upstash_redis", api.requests[0].variables["provider"])
	assert.Contains(t, stdout, "Your Redis database cache was destroyed")
}

func TestDestroyReturnsAPIError(t *testing.T) {
	api := &recordingAPI{}

	stdout, _, err := runCommand(t, newDestroy(), runDestroy, api, "--yes", "missing")
	require.ErrorIs(t, err, errNoReply)

	assert.NotContains(t, stdout, "was destroyed")
}

// Without --yes and without a terminal to confirm on, nothing is destroyed.
func TestDestroyWithoutYesSendsNothing(t *testing.T) {
	api := &recordingAPI{}

	_, _, err := runCommand(t, newDestroy(), runDestroy, api, "cache")
	require.ErrorContains(t, err, "yes flag must be specified")

	assert.Empty(t, api.requests)
}

func TestStatusSendsProvider(t *testing.T) {
	api := &recordingAPI{replies: map[string]string{"GetAddOn": `{"addOn": {
		"id": "addon_1",
		"name": "cache",
		"primaryRegion": "ord",
		"readRegions": ["iad", "lhr"],
		"publicUrl": "redis://default:password@fly-cache.upstash.io:6379",
		"options": {"eviction": true},
		"addOnPlan": {"displayName": "Pay-as-you-go"}
	}}`}}

	stdout, _, err := runCommand(t, newStatus(), runStatus, api, "cache")
	require.NoError(t, err)

	require.Len(t, api.requests, 1)
	assert.Equal(t, "GetAddOn", api.requests[0].op)
	assert.Equal(t, "cache", api.requests[0].variables["name"])
	assert.Equal(t, "upstash_redis", api.requests[0].variables["provider"])
	assert.Contains(t, stdout, "addon_1")
	assert.Contains(t, stdout, "iad,lhr")
	assert.Contains(t, stdout, "redis://default:password@fly-cache.upstash.io:6379")
}

func TestStatusReturnsAPIError(t *testing.T) {
	api := &recordingAPI{}

	stdout, _, err := runCommand(t, newStatus(), runStatus, api, "missing")
	require.ErrorIs(t, err, errNoReply)

	assert.Empty(t, stdout)
}

// update looks the database up first; a failed lookup stops it before any
// prompt or change.
func TestUpdateLooksUpWithProvider(t *testing.T) {
	api := &recordingAPI{}

	_, _, err := runCommand(t, newUpdate(), runUpdate, api, "cache")
	require.ErrorIs(t, err, errNoReply)

	require.Len(t, api.requests, 1)
	assert.Equal(t, "GetAddOn", api.requests[0].op)
	assert.Equal(t, "cache", api.requests[0].variables["name"])
	assert.Equal(t, "upstash_redis", api.requests[0].variables["provider"])
}
