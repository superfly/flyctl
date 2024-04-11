package imgsrc

import (
	"context"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
)

func getBuildToken(ctx context.Context, app *fly.AppCompact) (string, error) {
	return config.Tokens(ctx).Docker(), nil
}
