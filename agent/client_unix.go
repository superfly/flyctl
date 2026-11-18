//go:build !windows

package agent

import (
	"context"
	"net"
)

func (c *Client) dialContext(ctx context.Context) (conn net.Conn, err error) {
	return c.dialer.DialContext(ctx, c.network, c.address)
}

// bindsUnixSocket reports whether the agent listens on a unix socket.
func bindsUnixSocket() bool { return true }
