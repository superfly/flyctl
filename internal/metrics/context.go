package metrics

import "context"

type contextKey struct{}

func NewContext(ctx context.Context, cache Cache) context.Context {
	return context.WithValue(ctx, contextKey{}, cache)
}

func StoreFromContext(ctx context.Context) Cache {
	return ctx.Value(contextKey{}).(Cache)
}
