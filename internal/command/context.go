package command

import (
	"context"

	"github.com/spf13/cobra"
)

type contextKey struct{}

// NewContext derives a context that carries cmd from ctx.
func NewContext(ctx context.Context, cmd *cobra.Command) context.Context {
	return context.WithValue(ctx, contextKey{}, cmd)
}

// FromContext returns the Command ctx carries. It panics in case ctx carries
// no Command.
func FromContext(ctx context.Context) *cobra.Command {
	return ctx.Value(contextKey{}).(*cobra.Command)
}
