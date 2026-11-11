package mpg

import (
	"context"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

type tokensContextKey struct{}

// NewContextWithTokens derives a Context whose Managed Postgres requests
// authenticate with t instead of the session tokens in the config.
func NewContextWithTokens(ctx context.Context, t *tokens.Tokens) context.Context {
	return context.WithValue(ctx, tokensContextKey{}, t)
}

// TokensFromContext returns the tokens Managed Postgres requests should use:
// the override set by NewContextWithTokens, or else the config's tokens.
func TokensFromContext(ctx context.Context) *tokens.Tokens {
	if t, ok := ctx.Value(tokensContextKey{}).(*tokens.Tokens); ok && t != nil {
		return t
	}

	return config.Tokens(ctx)
}
