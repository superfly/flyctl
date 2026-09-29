//go:build !js

package wg

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/conn"
)

// echoingLink connects a proxy to a fake gateway that relays every frame
// straight back, and returns the proxy plus the gateway.
func echoingLink(t *testing.T, answers int) (*WsWgProxy, *fakeTokenGateway, context.CancelFunc) {
	t.Helper()

	gw, _ := testKeypair(t)
	pubkeys := make([]string, answers)
	results := make([]tokenResult, answers)
	for i := range answers {
		pubkeys[i] = gw
		results[i] = tokenResult{Type: "ok", PeerIP: "fdaa:0:18:a7b:1:1111:2222:3302", DNS: "fdaa:0:18::3", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	}

	gateway := newFakeTokenGateway(t, pubkeys, results)
	gateway.echo = true

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pub, _ := testKeypair(t)
	wswg, err := websocketConnectAuth(ctx, ctx, gateway.endpoint(), &TokenAuth{
		Token:   func() string { return "FlyV1 fm2_test" },
		Pubkey:  pub,
		OrgSlug: "test-org",
	})
	require.NoError(t, err)
	wswg.start(ctx, gateway.endpoint())

	<-gateway.ready

	return wswg, gateway, cancel
}

// receiveOne runs a bind's receive function once with a deadline.
func receiveOne(t *testing.T, recv conn.ReceiveFunc) ([]byte, error) {
	t.Helper()

	type result struct {
		pkt []byte
		err error
	}
	ch := make(chan result, 1)

	go func() {
		bufs := [][]byte{make([]byte, maxFrame)}
		sizes := []int{0}
		eps := []conn.Endpoint{nil}

		n, err := recv(bufs, sizes, eps)
		if err != nil {
			ch <- result{nil, err}

			return
		}
		require.Equal(t, 1, n)
		assert.Equal(t, wsEndpoint{}, eps[0])
		ch <- result{bufs[0][:sizes[0]], nil}
	}()

	select {
	case r := <-ch:
		return r.pkt, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a frame")

		return nil, nil
	}
}

func TestWsBindRelaysFrames(t *testing.T) {
	wswg, _, _ := echoingLink(t, 1)

	bind := wswg.bind()
	fns, _, err := bind.Open(0)
	require.NoError(t, err)
	require.Len(t, fns, 1)

	_, _, err = bind.Open(0)
	assert.ErrorIs(t, err, conn.ErrBindAlreadyOpen)

	for _, want := range [][]byte{[]byte("hello"), make([]byte, 1420), []byte{0}} {
		require.NoError(t, bind.Send([][]byte{want}, wsEndpoint{}))

		got, err := receiveOne(t, fns[0])
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	// a zero-length frame is a keepalive, not a packet: the next real frame
	// is what comes out
	require.NoError(t, wswg.wsWrite(wswg.wsConn, make([]byte, 4)))
	require.NoError(t, bind.Send([][]byte{[]byte("after keepalive")}, wsEndpoint{}))
	got, err := receiveOne(t, fns[0])
	require.NoError(t, err)
	assert.Equal(t, []byte("after keepalive"), got)

	assert.ErrorIs(t, bind.Send([][]byte{make([]byte, maxFrame+1)}, wsEndpoint{}), errFrameTooLarge)
}

func TestWsBindCloseUnblocksReceive(t *testing.T) {
	wswg, _, _ := echoingLink(t, 1)

	bind := wswg.bind()
	fns, _, err := bind.Open(0)
	require.NoError(t, err)

	errs := make(chan error, 1)
	go func() {
		_, err := receiveOne(t, fns[0])
		errs <- err
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, bind.Close())
	require.NoError(t, bind.Close(), "close is idempotent")

	select {
	case err := <-errs:
		assert.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("receive didn't return after close")
	}

	assert.ErrorIs(t, bind.Send([][]byte{[]byte("x")}, wsEndpoint{}), net.ErrClosed)

	// a second bind on the same link picks up where the first left off
	second := wswg.bind()
	fns, _, err = second.Open(0)
	require.NoError(t, err)
	require.NoError(t, second.Send([][]byte{[]byte("again")}, wsEndpoint{}))
	got, err := receiveOne(t, fns[0])
	require.NoError(t, err)
	assert.Equal(t, []byte("again"), got)
}

func TestWsBindSurvivesReconnect(t *testing.T) {
	wswg, gateway, _ := echoingLink(t, 2)

	bind := wswg.bind()
	fns, _, err := bind.Open(0)
	require.NoError(t, err)

	// one receiver for the whole test, as a device would run
	frames := make(chan string, 16)
	go func() {
		for {
			bufs := [][]byte{make([]byte, maxFrame)}
			sizes := []int{0}
			eps := []conn.Endpoint{nil}

			n, err := fns[0](bufs, sizes, eps)
			if err != nil || n != 1 {
				return
			}
			frames <- string(bufs[0][:sizes[0]])
		}
	}()

	require.NoError(t, bind.Send([][]byte{[]byte("before")}, wsEndpoint{}))
	select {
	case got := <-frames:
		assert.Equal(t, "before", got)
	case <-time.After(5 * time.Second):
		t.Fatal("no frame before the drop")
	}

	gateway.drop(0)
	<-gateway.ready

	// sends into the gap are dropped, not errors; once the new connection
	// is up they flow again through the same bind
	deadline := time.Now().Add(10 * time.Second)
	for {
		require.NoError(t, bind.Send([][]byte{[]byte("after")}, wsEndpoint{}))

		select {
		case got := <-frames:
			assert.Equal(t, "after", got)

			return
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			t.Fatal("no frame after the reconnect")
		}
	}
}

func TestWsBindLifetimeEndsReceive(t *testing.T) {
	wswg, _, cancel := echoingLink(t, 1)

	bind := wswg.bind()
	fns, _, err := bind.Open(0)
	require.NoError(t, err)

	cancel()

	_, err = receiveOne(t, fns[0])
	assert.True(t, errors.Is(err, net.ErrClosed))
}
