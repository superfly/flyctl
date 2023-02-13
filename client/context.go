package client

import "context"

type contextKey struct{}

// NewContext derives a Context that carries c from ctx.
func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext returns the Client ctx carries. It panics in case ctx carries
// no Client.
func FromContext(ctx context.Context) *Client {
	return ctx.Value(contextKey{}).(*Client)
}
