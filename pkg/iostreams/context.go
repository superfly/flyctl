package iostreams

import "context"

type contextKey struct{}

// NewContext derives a context that carries io from ctx.
func NewContext(ctx context.Context, io *IOStreams) context.Context {
	return context.WithValue(ctx, contextKey{}, io)
}

// FromContext returns the IOStreams ctx carries. It panics in case ctx carries
// no IOStreams.
func FromContext(ctx context.Context) *IOStreams {
	return ctx.Value(contextKey{}).(*IOStreams)
}
