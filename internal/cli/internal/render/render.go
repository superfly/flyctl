package render

import (
	"context"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/pkg/iostreams"
)

// Presentable renders p into the appropriate io streams of ctx.
func Presentable(ctx context.Context, p presenters.Presentable, opts presenters.Options) error {
	presenter := &presenters.Presenter{
		Item: p,
		Out:  iostreams.FromContext(ctx).Out,
		Opts: opts,
	}

	return presenter.Render()
}
