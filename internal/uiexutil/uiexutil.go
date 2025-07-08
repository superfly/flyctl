package uiexutil

import (
	"context"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/uiex"
)

func NewClientWithOptions(ctx context.Context, opts uiex.NewClientOpts) (*uiex.Client, error) {
	if opts.Tokens == nil {
		opts.Tokens = config.Tokens(ctx)
	}

	if v := logger.MaybeFromContext(ctx); v != nil && opts.Logger == nil {
		opts.Logger = v
	}
	return uiex.NewWithOptions(ctx, opts)
}
