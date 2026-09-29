//go:build js

package wg

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/coder/websocket"
)

// A browser can't set headers on a websocket upgrade, so the macaroon
// always rides in the auth packet.
const wsAuthHeaders = false

// dialWebsocket dials the gateway through the browser's WebSocket. Browsers
// enforce UTF-8 on text frames, which WireGuard packets aren't, so the
// frames have to be binary; the wswg-binary subprotocol asks the gateway to
// answer in kind (it accepts binary frames regardless).
var dialWebsocket = func(dialCtx, lifetimeCtx context.Context, endpoint string, _ bool, _ string) (net.Conn, error) {
	rurl := websocketURL(endpoint)

	log.Printf("(re-)connecting to %s", rurl)

	ws, _, err := websocket.Dial(dialCtx, rurl, &websocket.DialOptions{ // nolint: bodyclose
		Subprotocols: []string{WsBinarySubprotocol},
	})
	if err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}

	if got := ws.Subprotocol(); got != WsBinarySubprotocol {
		_ = ws.Close(websocket.StatusProtocolError, "binary framing required")

		return nil, fmt.Errorf("websocket: gateway did not accept %s framing (got %q); gateway too old?", WsBinarySubprotocol, got)
	}

	return websocket.NetConn(lifetimeCtx, ws, websocket.MessageBinary), nil
}
