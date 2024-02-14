//go:build !windows

package agent

import (
	"context"
	"net"
)

func (c *Client) dialContext(ctx context.Context) (conn net.Conn, err error) {
	return c.dialer.DialContext(ctx, c.network, c.address)
}
