//go:build !windows

package agent

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/agent/internal/proto"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/wg"
)

func TestPersonalDialerAndPingerKeepEstablishedIdentity(t *testing.T) {
	// The same client can be used with different credential contexts. A single
	// mutable mapping from "personal" to a slug would send A's later dial to B.
	requests := make(chan string, 16)
	c := personalTestClient(t, func(account string) *wg.WireGuardState { return &wg.WireGuardState{Org: "personal-" + account} }, requests)
	ctx := func(account string) context.Context {
		return config.NewContext(context.Background(), &config.Config{Tokens: tokens.Parse("fo1_" + account)})
	}
	a, err := c.Dialer(ctx("a"), "personal", "blue")
	require.NoError(t, err)
	b, err := c.Dialer(ctx("b"), "personal", "blue")
	require.NoError(t, err)
	for _, d := range []Dialer{a, b, a} {
		conn, err := d.DialContext(context.Background(), "tcp", "example.internal:80")
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}
	assertRequest := func(want string) {
		t.Helper()
		select {
		case got := <-requests:
			require.Equal(t, want, got)
		case <-time.After(5 * time.Second):
			t.Fatal("missing agent request")
		}
	}
	for _, account := range []string{"a", "b", "a"} {
		assertRequest("connect personal-" + account + " example.internal:80 0 blue")
	}
	for _, account := range []string{"a", "b"} {
		p, err := c.Pinger(ctx(account), "personal", "")
		require.NoError(t, err)
		require.NoError(t, p.Close())
		// ping6 has no acknowledgement; observe this request before the next.
		assertRequest("ping6 personal-" + account)
	}

}

func TestPersonalPollingUsesEstablishedIdentity(t *testing.T) {
	requests := make(chan string, 2)
	c := personalTestClient(t, func(string) *wg.WireGuardState { return &wg.WireGuardState{Org: "personal-a"} }, requests)
	ctx := config.NewContext(context.Background(), &config.Config{Tokens: tokens.Parse("fo1_a")})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("quiet", true, "")
	ctx = flag.NewContext(ctx, flags)
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{})
	d, err := c.ConnectToTunnel(ctx, "personal", "blue", true)
	require.NoError(t, err)
	require.NoError(t, c.WaitForDNS(ctx, d, "personal", "example.internal", "blue"))
	require.Equal(t, "probe personal-a blue", <-requests)
	require.Equal(t, "resolve personal-a example.internal blue", <-requests)
}

func TestPersonalOldAgentKeepsWorking(t *testing.T) {
	for _, state := range []*wg.WireGuardState{nil, {}, {Org: "personal"}} {
		requests := make(chan string, 2)
		c := personalTestClient(t, func(string) *wg.WireGuardState { return state }, requests)
		ctx := config.NewContext(context.Background(), &config.Config{Tokens: tokens.Parse("fo1_a")})
		d, err := c.Dialer(ctx, "personal", "")
		require.NoError(t, err)
		conn, err := d.DialContext(ctx, "tcp", "example.internal:80")
		require.NoError(t, err)
		conn.Close()
		require.Equal(t, "connect personal example.internal:80 0 ", <-requests)
		p, err := c.Pinger(ctx, "personal", "")
		require.NoError(t, err)
		p.Close()
		require.Equal(t, "ping6 personal", <-requests)
	}
}

func personalTestClient(t *testing.T, response func(string) *wg.WireGuardState, requests chan<- string) *Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "pa-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	listener, err := net.Listen("unix", filepath.Join(dir, "agent.sock"))
	require.NoError(t, err)
	var workers sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				packet, err := proto.Read(conn)
				if err != nil {
					t.Error(err)
					return
				}
				account := ""
				if strings.HasPrefix(string(packet), "set-token str fo1_") {
					account = strings.TrimPrefix(string(packet), "set-token str fo1_")
					if err := proto.Write(conn, "ok"); err != nil {
						t.Error(err)
						return
					}
					packet, err = proto.Read(conn)
					if err != nil {
						t.Error(err)
						return
					}
				}
				if strings.HasPrefix(string(packet), "establish ") {
					body, err := json.Marshal(EstablishResponse{WireGuardState: response(account)})
					if err != nil {
						t.Error(err)
						return
					}
					if err := proto.Write(conn, "ok", string(body)); err != nil {
						t.Error(err)
					}

					return
				}
				requests <- string(packet)
				if strings.HasPrefix(string(packet), "probe ") {
					if err := proto.Write(conn, "ok"); err != nil {
						t.Error(err)
					}
					return
				}
				if strings.HasPrefix(string(packet), "resolve ") {
					if err := proto.Write(conn, "ok", "fdaa::1"); err != nil {
						t.Error(err)
					}
					return
				}
				if strings.HasPrefix(string(packet), "connect ") {
					if err := proto.Write(conn, "ok"); err != nil {
						t.Error(err)
						return
					}
				}
				io.Copy(io.Discard, conn)
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done; workers.Wait() })

	return newClient("unix", listener.Addr().String())
}
