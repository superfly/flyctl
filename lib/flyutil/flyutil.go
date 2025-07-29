package flyutil

import (
	"context"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/lib/buildinfo"
	"github.com/superfly/flyctl/lib/logger"
)

func NewClientFromOptions(ctx context.Context, opts fly.ClientOptions) *fly.Client {
	if opts.Name == "" {
		opts.Name = buildinfo.Name()
	}
	if opts.Version == "" {
		opts.Version = buildinfo.Version().String()
	}
	if v := logger.MaybeFromContext(ctx); v != nil && opts.Logger == nil {
		opts.Logger = v
	}
	return fly.NewClientFromOptions(opts)
}
