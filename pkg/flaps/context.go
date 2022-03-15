package flaps

import "context"

type contextKey struct{}

func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

func FromContext(ctx context.Context) *Client {
	return ctx.Value(contextKey{}).(*Client)
}
