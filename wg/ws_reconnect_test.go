package wg

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// Reject the requested number of upgrades after the initial connection.
// Accepted connections echo frames, exercising both real relay directions.
func reconnectGateway(t *testing.T, failures int) (string, func() []time.Time) {
	t.Helper()
	var mu sync.Mutex
	var attempts []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := len(attempts)
		attempts = append(attempts, time.Now())
		mu.Unlock()
		if n > 0 && n <= failures {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		conn := websocket.NetConn(r.Context(), ws, websocket.MessageText)
		defer conn.Close()
		var magic [4]byte
		if _, err := io.ReadFull(conn, magic[:]); err != nil {
			return
		}
		if binary.BigEndian.Uint32(magic[:]) != 0x2FACED77 {
			t.Error("incorrect legacy magic")
			return
		}
		_, _ = io.Copy(conn, conn)
	}))
	t.Cleanup(server.Close)

	return server.Listener.Addr().String(), func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Time(nil), attempts...)
	}
}

func waitReset(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("old connection error blocked after replacement or cancellation")
	}
}

func TestWebsocketReconnectRelaysTraffic(t *testing.T) {
	endpoint, attempts := reconnectGateway(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p, err := websocketConnectAuth(ctx, ctx, endpoint, nil)
	require.NoError(t, err)
	old := p.wsConn
	p.start(ctx, endpoint)

	udp, err := net.DialUDP("udp", nil, p.plugConn.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	defer udp.Close()
	echo := func() {
		t.Helper()
		require.NoError(t, udp.SetDeadline(time.Now().Add(time.Second)))
		_, err := udp.Write([]byte("healthy traffic"))
		require.NoError(t, err)
		var packet [100]byte
		n, err := udp.Read(packet[:])
		require.NoError(t, err)
		require.Equal(t, "healthy traffic", string(packet[:n]))
	}
	echo()

	// Model simultaneous read, write and keepalive failures from the same
	// socket. They must coalesce, without independent rate-limit waits.
	done := make(chan struct{}, 3)
	for range 3 {
		go func() {
			p.resetConn(ctx, old, io.EOF)
			done <- struct{}{}
		}()
	}
	require.NoError(t, old.Close())
	waitFor(t, "replacement after failed upgrades", func() bool {
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.wsConn != old
	})
	for range 3 {
		waitReset(t, done)
	}
	echo()

	// A report already in flight can arrive after Connect installed the
	// replacement. The owner must reject it, not reset the healthy socket.
	select {
	case p.reset <- old:
	case <-time.After(time.Second):
		t.Fatal("reconnect owner stopped receiving reports")
	}
	time.Sleep(4 * reconnectInterval)
	echo()
	got := attempts()
	require.Len(t, got, 4, "one initial dial, two failures and one replacement")
	for i := 1; i < len(got); i++ {
		require.GreaterOrEqual(t, got[i].Sub(got[i-1]), reconnectInterval,
			"duplicate errors must not accelerate reconnect attempts")
	}
}

// Done is evaluated when resetConn enters its cancellable wait, after it
// snapshots the connection. This makes replacement-before-report deterministic.
type resetWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *resetWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })

	return c.Context.Done()
}

func TestResetConnWakesOnReplacementBeforeReport(t *testing.T) {
	endpoint, attempts := reconnectGateway(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := websocketConnectAuth(ctx, ctx, endpoint, nil)
	require.NoError(t, err)
	defer p.close()
	old := p.wsConn
	reportCtx := &resetWaitContext{Context: ctx, waiting: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		p.resetConn(reportCtx, old, io.EOF)
		close(done)
	}()
	waitReset(t, reportCtx.waiting)
	// No reset receiver exists. Replacement must release the reporter even
	// when it has not managed to enqueue its request.
	p.lock.Lock()
	err = p.Connect(ctx, ctx, endpoint)
	p.lock.Unlock()
	require.NoError(t, err)
	waitReset(t, done)
	require.Len(t, attempts(), 2)
}

func TestResetConnCancellation(t *testing.T) {
	for _, receive := range []bool{false, true} {
		name := "before report"
		if receive {
			name = "after report"
		}
		t.Run(name, func(t *testing.T) {
			p, err := NewWsWgProxy()
			require.NoError(t, err)
			conn, peer := net.Pipe()
			p.wsConn = conn
			defer p.close()
			defer peer.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reportCtx := &resetWaitContext{Context: ctx, waiting: make(chan struct{})}
			done := make(chan struct{})
			go func() {
				p.resetConn(reportCtx, conn, io.EOF)
				close(done)
			}()
			waitReset(t, reportCtx.waiting)
			if receive {
				select {
				case failed := <-p.reset:
					require.Equal(t, conn, failed)
				case <-time.After(time.Second):
					t.Fatal("missing reset report")
				}
				select {
				case <-done:
					t.Fatal("reporter must wait rather than spin on the dead socket")
				default:
				}
			}
			cancel()
			waitReset(t, done)
		})
	}
}
