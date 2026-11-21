package wireguard

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/wg"
)

// runList runs list for my-org with one API peer, and the agent answering
// with tunnels or err. It returns stdout and stderr.
func runList(t *testing.T, jsonOutput bool, tunnels []wg.TunnelStatus, agentErr error) (string, string) {
	t.Helper()

	return runListFor(t, &uiex.Organization{Slug: "my-org"}, "my-org", jsonOutput, tunnels, agentErr)
}

// runListFor is runList for org, expecting the agent to be asked for
// agentSlug.
func runListFor(t *testing.T, org *uiex.Organization, agentSlug string, jsonOutput bool, tunnels []wg.TunnelStatus, agentErr error) (string, string) {
	t.Helper()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	require.NoError(t, fs.Parse([]string{"my-org"}))

	ios, _, out, errOut := iostreams.Test()

	ctx := flag.NewContext(context.Background(), fs)
	ctx = iostreams.NewContext(ctx, ios)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) { return org, nil },
	})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GetWireGuardPeersFunc: func(context.Context, string) ([]*fly.WireGuardPeer, error) {
			return []*fly.WireGuardPeer{{ID: "p1", Pubkey: "k1", Region: "ams", Name: "laptop", Peerip: "fdaa::1"}}, nil
		},
	})

	orig := agentTunnels
	t.Cleanup(func() { agentTunnels = orig })
	agentTunnels = func(_ context.Context, slug string) ([]wg.TunnelStatus, error) {
		assert.Equal(t, agentSlug, slug)

		return tunnels, agentErr
	}

	require.NoError(t, runWireguardList(ctx))

	return out.String(), errOut.String()
}

var agentHeld = []wg.TunnelStatus{
	{Org: "my-org", Token: true, Name: "token-9f3c21aa", Pubkey: "k2", PeerIP: "fdaa::2", Endpoint: "203.0.113.7:51820"},
	{Org: "my-org", Name: "interactive-x", Pubkey: "k3", PeerIP: "fdaa::3", Endpoint: "198.51.100.1:51820"},
}

func TestListShowsTokenTunnels(t *testing.T) {
	out, errOut := runList(t, false, agentHeld, nil)

	assert.Empty(t, errOut)
	assert.Contains(t, out, "laptop")
	assert.Contains(t, out, "Token-mode tunnels")
	assert.Contains(t, out, "token-9f3c21aa")
	assert.Contains(t, out, "203.0.113.7:51820")

	// legacy tunnels are the API's peers; the API table has them
	assert.NotContains(t, out, "interactive-x")
}

func TestListJSONAppendsTokenTunnels(t *testing.T) {
	out, _ := runList(t, true, agentHeld, nil)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 2)

	assert.Equal(t, "laptop", rows[0]["Name"])
	assert.Equal(t, map[string]any{
		"ID": "", "Pubkey": "k2", "Region": "", "Name": "token-9f3c21aa", "Peerip": "fdaa::2", "Endpoint": "203.0.113.7:51820",
	}, rows[1])
}

func TestListWithoutTokenTunnelsIsUnchanged(t *testing.T) {
	out, errOut := runList(t, false, nil, agent.ErrAgentNotRunning)

	assert.Empty(t, errOut, "no agent is not worth a warning")
	assert.Contains(t, out, "laptop")
	assert.NotContains(t, out, "Token-mode")

	out, _ = runList(t, true, nil, agent.ErrAgentNotRunning)

	var peers []*fly.WireGuardPeer
	require.NoError(t, json.Unmarshal([]byte(out), &peers))
	require.Len(t, peers, 1)
}

func TestListAgentTroubleStillListsPeers(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "old agent", err: agent.ErrUnsupportedCommand, want: "fly agent restart"},
		{name: "broken agent", err: errors.New("boom"), want: "boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut := runList(t, false, nil, tc.err)

			assert.Contains(t, out, "laptop")
			assert.Contains(t, errOut, tc.want)
		})
	}
}

// The agent keys the personal org's tunnels by its real slug, while uiex
// calls it "personal".
func TestListAsksAgentForPersonalOrgByRealSlug(t *testing.T) {
	out, _ := runListFor(t, &uiex.Organization{Slug: "personal", RawSlug: "jane-doe-123"}, "jane-doe-123", false, agentHeld, nil)

	assert.Contains(t, out, "token-9f3c21aa")
}
