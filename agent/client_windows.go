//go:build windows

package agent

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func (c *Client) dialContext(ctx context.Context) (conn net.Conn, err error) {
	if UseUnixSockets() {
		return c.dialer.DialContext(ctx, c.network, c.address)
	}

	pipe, err := PipeName()
	if err != nil {
		return nil, err
	}

	return winio.DialPipeContext(ctx, pipe)
}
