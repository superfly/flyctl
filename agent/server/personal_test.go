package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/agent/internal/proto"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/wg"
)

// Exercise independent protocol sessions, as used by two CLI invocations,
// against one agent. All API and WireGuard endpoints are local test fixtures.
func TestPersonalAccountsUseDistinctTunnels(t *testing.T) {
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "RUN_ID"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Setenv("WSWG", "")
	t.Setenv("FLY_AGENT_WG_REGION", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
	ctx := config.NewContext(context.Background(), &config.Config{})
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	at, err := latestChange(path)
	require.NoError(t, err)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(request.Query, "organizations(") {
			account := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer fo1_")
			raw := "personal-" + account
			fmt.Fprintf(w, `{"data":{"organizations":{"nodes":[{"id":%q,"slug":"personal","rawSlug":%q,"type":"PERSONAL","internalNumericId":"1"},{"id":"shared-id","slug":"shared","rawSlug":"shared","type":"SHARED","internalNumericId":"2"}]}}}`, account, raw)
		} else {
			// A token viewer makes feature-flag evaluation keep local legacy mode.
			io.WriteString(w, `{"data":{"viewer":{"__typename":"Macaroon"}}}`)
		}
	}))
	defer api.Close()
	fly.SetBaseURL(api.URL)
	defer fly.SetBaseURL("")

	peer := func(account string) *wg.WireGuardState {
		pub, priv := wireguard.C25519pair()
		gateway, _ := wireguard.C25519pair()
		return &wg.WireGuardState{Org: "personal-" + account, Name: account, LocalPublic: pub, LocalPrivate: priv,
			Peer: fly.CreatedWireGuardPeer{Pubkey: gateway, Peerip: "fdaa:0:1::2", Endpointip: "127.0.0.1"}}
	}
	a, b := peer("a"), peer("b")
	shared := peer("shared")
	shared.Org = "shared"
	// Include the ambiguous pre-upgrade entry as well as canonical entries.
	viper.Set(flyctl.ConfigWireGuardState, wg.States{"personal": a, "personal-a": a, "personal-b": b, "shared": shared})
	srv := &server{Options: Options{ConfigFile: path, Logger: log.New(io.Discard, "", 0)},
		runCtx: ctx, currentChange: at, tokens: tokens.Parse("fo1_a"),
		tunnels: make(map[tunnelKey]*wg.Tunnel), cancelTokenMonitoring: func() {}}
	defer func() {
		for _, tunnel := range srv.tunnels {
			require.NoError(t, tunnel.Close())
		}
	}()
	establish := func(account, slug string) *agent.EstablishResponse {
		t.Helper()
		reply := personalCommand(t, ctx, srv, "fo1_"+account, "establish", slug, "")
		require.True(t, strings.HasPrefix(reply, "ok "), reply)
		var response agent.EstablishResponse
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(reply, "ok ")), &response))

		return &response
	}
	require.Equal(t, "a", establish("a", "personal").WireGuardState.Name)
	require.Equal(t, "b", establish("b", "personal").WireGuardState.Name)
	require.Equal(t, "a", establish("a", "personal-a").WireGuardState.Name)
	require.Equal(t, "b", establish("b", "personal-b").WireGuardState.Name)
	sharedA := establish("a", "shared")
	sharedB := establish("b", "shared")
	require.Equal(t, sharedA, sharedB)

	// Control requests still using the alias resolve it with their own tokens.
	for _, account := range []string{"a", "b"} {
		session := &session{srv: srv, tokens: tokens.Parse("fo1_" + account)}
		tunnel, err := session.tunnelFor(ctx, "personal", "")
		require.NoError(t, err)
		require.Equal(t, account, tunnel.State.Name)
	}

	// A transport request carries no credentials, so the alias must not be
	// resolved for it: the agent's own account (a) is not necessarily the caller.
	require.Equal(t, "err tunnel unavailable", personalCommand(t, ctx, srv, "", "connect", "personal", "[fdaa:0:1::9]:80", "200", ""))
	require.Equal(t, "err tunnel unavailable", personalCommand(t, ctx, srv, "", "ping6", "personal"))
}

func personalCommand(t *testing.T, ctx context.Context, srv *server, token, verb string, args ...string) string {
	t.Helper()
	client, conn := net.Pipe()
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	done := make(chan struct{})
	go func() { defer close(done); runSession(ctx, srv, conn, 1) }()
	defer func() { client.Close(); <-done }()
	if token != "" {
		require.NoError(t, proto.Write(client, "set-token", "str", token))
		reply, err := proto.Read(client)
		require.NoError(t, err)
		require.Equal(t, "ok", string(reply))
	}
	var frame bytes.Buffer
	require.NoError(t, proto.Write(&frame, verb, args...))
	_, err := client.Write(frame.Bytes())
	require.NoError(t, err)
	reply, err := proto.Read(client)
	require.NoError(t, err)

	return string(reply)
}
