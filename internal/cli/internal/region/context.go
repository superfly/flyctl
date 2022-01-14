package region

import "context"

type contextKey struct{}

func WithRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, contextKey{}, region)
}

func FromContext(ctx context.Context) string {
	return ctx.Value(contextKey{}).(string)
}
