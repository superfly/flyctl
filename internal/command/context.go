package command

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command_context"
)

// NewContext derives a context that carries cmd from ctx.
func NewContext(ctx context.Context, cmd *cobra.Command) context.Context {
	// uses command_context.go so there isn't a dependency cycle with flaps
	return command_context.NewContext(ctx, cmd)
}

// FromContext returns the Command ctx carries. It panics in case ctx carries
// no Command.
func FromContext(ctx context.Context) *cobra.Command {
	return command_context.FromContext(ctx)
}
