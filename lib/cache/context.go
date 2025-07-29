package cache

import "context"

type contextKey struct{}

// NewContext derives a context that carries c from ctx.
func NewContext(ctx context.Context, c Cache) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext returns the Cache ctx carries. It panics in case ctx carries
// no Cache.
func FromContext(ctx context.Context) Cache {
	return ctx.Value(contextKey{}).(Cache)
}
