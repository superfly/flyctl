package profile

import "context"

type contextKey struct{}

// NewContext derives a context that carries res from ctx.
func NewContext(ctx context.Context, res Resolution) context.Context {
	return context.WithValue(ctx, contextKey{}, res)
}

// FromContext returns the Resolution ctx carries, along with whether ctx
// carried one at all.
func FromContext(ctx context.Context) (Resolution, bool) {
	res, ok := ctx.Value(contextKey{}).(Resolution)

	return res, ok
}
