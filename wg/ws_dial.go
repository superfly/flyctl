//go:build !js

package wg

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/coder/websocket"
)

// wsAuthHeaders reports whether dialWebsocket can send the macaroon as the
// upgrade request's Authorization header. Native builds can; a browser
// can't set headers on a websocket, so it goes in the auth packet instead.
const wsAuthHeaders = true

// dialWebsocket dials the gateway's wswg endpoint and wraps it as a
// net.Conn bound to lifetimeCtx. A non-empty authorization goes out as the
// upgrade request's Authorization header. A variable so tests can point it
// at a plain-text local server.
var dialWebsocket = func(dialCtx, lifetimeCtx context.Context, endpoint string, verifyTLS bool, authorization string) (net.Conn, error) {
	rurl := websocketURL(endpoint)

	log.Printf("(re-)connecting to %s", rurl)

	header := http.Header{
		"Origin": []string{rurl},
	}
	if authorization != "" {
		header.Set("Authorization", authorization)
	}

	ws, _, err := websocket.Dial(dialCtx, rurl, &websocket.DialOptions{ // nolint: bodyclose
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				// Legacy gateways serve a self-signed cert; the tunnel traffic is
				// WG-encrypted anyway. Token gateways have a real cert for their
				// hostname and we hand them a macaroon, so verify those.
				TLSClientConfig: &tls.Config{ // skipcq: GO-S1020
					InsecureSkipVerify: !verifyTLS, // skipcq: GSC-G402
				},
			},
		},
		HTTPHeader: header,
	})
	if err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}

	// Text frames, for gateways that predate binary framing. The frames
	// carry arbitrary bytes, which only works because neither side
	// validates UTF-8; browsers do, hence ws_dial_js.go.
	return websocket.NetConn(lifetimeCtx, ws, websocket.MessageText), nil
}
