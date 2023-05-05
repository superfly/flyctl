package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/websocket"
)

var websocketConn *websocket.Conn
var websocketMu    sync.Mutex
var done           sync.WaitGroup

func websocketURL(cfg *config.Config) (*url.URL, error) {
	url, err := url.Parse(cfg.MetricsBaseURL)
	if err != nil {
		return nil, err
	}

	switch url.Scheme {
	case "http":
		url.Scheme = "ws"
	case "https":
		url.Scheme = "wss"
	}

	return url.JoinPath("/socket"), nil
}

func connectWebsocket(ctx context.Context) (*websocket.Conn, error) {
	cfg := config.FromContext(ctx)

	url, err := websocketURL(cfg)
	if err != nil {
		return nil, err
	}

	// websockets require an origin url - this doesn't make sense in flyctl's
	// case, so let's just reuse the connection url as the origin.
	origin := url

	wsCfg, err := websocket.NewConfig(url.String(), origin.String())
	if err != nil {
		return nil, err
	}

	authToken, err := getMetricsToken(ctx)
	if err != nil {
		return nil, err
	}

	wsCfg.Header.Set("Authorization", authToken)
	wsCfg.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Version().String()))

	return websocket.DialConfig(wsCfg)
}

func getWebsocketConn(ctx context.Context) *websocket.Conn {
	websocketMu.Lock()
	defer websocketMu.Unlock()

	if websocketConn == nil {
		conn, err := connectWebsocket(ctx)
		if err != nil {
			// failed to connect metrics websocket, nothing we can do
			terminal.Debugf("failed to connect metrics websocket: %v\n", err)
			return nil
		}
		websocketConn = conn
	}
	return websocketConn
}

type websocketMessage struct {
	Metric  string          `json:"m"`
	Payload json.RawMessage `json:"p"`
}

func rawSendImpl(ctx context.Context, metricSlug string, payload json.RawMessage) error {
	conn := getWebsocketConn(ctx)
	if conn == nil {
		// returning nil here is fine since getWebsocketConn returning
		// nil means we have already logged an error
		return nil
	}

	message := websocketMessage {
		Metric: metricSlug,
		Payload: payload,
	}

	return websocket.JSON.Send(conn, &message)
}

func handleErr(err error) {
	if err == nil {
		return
	}
	// TODO(ali): Should this ping sentry when it fails?
	terminal.Debugf("metrics error: %v\n", err)
}

func rawSend(parentCtx context.Context, metricSlug string, payload json.RawMessage) {
	if !shouldSendMetrics(parentCtx) {
		return
	}

	done.Add(1)
	go func() {
		defer done.Done()
		handleErr(rawSendImpl(parentCtx, metricSlug, payload))
	}()
}

func shouldSendMetrics(ctx context.Context) bool {
	cfg := config.FromContext(ctx)

	// never send metrics to the production collector from dev builds
	if buildinfo.IsDev() && cfg.MetricsBaseURLIsProduction() {
		return false
	}

	return true
}

func FlushPending() {
	// this just waits for metrics to hit write(2) on the websocket connection
	// there is no need to wait on a response from the collector
	done.Wait()
}
