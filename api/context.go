package api

import "context"

type contextKey string

const (
	contextKeyAuthorization = contextKey("authorization")
	contextKeyRequestStart  = contextKey("RequestStart")
)

// WithAuthorizationHeader returns a context that instructs the client to use
// the specified Authorization header value.
func WithAuthorizationHeader(ctx context.Context, hdr string) context.Context {
	return context.WithValue(ctx, contextKeyAuthorization, hdr)
}
