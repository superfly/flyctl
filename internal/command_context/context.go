package command_context

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

// FromContextOrNil returns the Command ctx carries, or nil in case ctx carries
// none. It exists for command preparers, which also run on paths that never
// set up a command, such as shell completion.
func FromContextOrNil(ctx context.Context) *cobra.Command {
	cmd, _ := ctx.Value(contextKey{}).(*cobra.Command)

	return cmd
}

// HasAnnotation reports whether the Command ctx carries is annotated with key.
func HasAnnotation(ctx context.Context, key string) bool {
	cmd := FromContextOrNil(ctx)
	if cmd == nil {
		return false
	}

	_, ok := cmd.Annotations[key]

	return ok
}
