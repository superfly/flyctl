package agent

import (
	"context"
)

type contextKey struct{}

func DialerWithContext(ctx context.Context, dialer Dialer) context.Context {
	return context.WithValue(ctx, contextKey{}, dialer)
}

func DialerFromContext(ctx context.Context) Dialer {
	return ctx.Value(contextKey{}).(Dialer)
}
