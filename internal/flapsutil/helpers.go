package flapsutil

import (
	"context"
	"fmt"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"

	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
)

// SetClient builds a flaps client for appName and stores it in a new context which is returned.
// It also returns the flaps client and the AppCompact for appName, which it must lookup.
// On error the old context is returned along with the error.
// The context must already have the flyutil client set.
func SetClient(ctx context.Context, appName string) (context.Context, FlapsClient, *fly.AppCompact, error) {
	client := flyutil.ClientFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return ctx, nil, nil, fmt.Errorf("get app %s: %w", appName, err)
	}

	flapsClient, err := NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		err = flyerr.GenericErr{
			Err: fmt.Sprintf("could not create flaps client: %v", err),
		}
		return ctx, flapsClient, app, err
	}

	ctx = NewContextWithClient(ctx, flapsClient)
	return ctx, flapsClient, app, nil
}
