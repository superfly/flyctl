package imgsrc

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/proxy"
	"golang.org/x/sync/errgroup"
)

func NixSourceBuild(ctx context.Context) (img *DeploymentImage, err error) {
	client := client.FromContext(ctx).API()
	var eg *errgroup.Group

	ports := []string{"2003", "2000"}
	app, err := client.GetApp(ctx, "fly-nix-builder")
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	eg, ctx = errgroup.WithContext(ctx)
	proxyContext, closeProxy := context.WithCancel(ctx)

	// run a proxy from local 30800 to remote 370 (rsyncd)
	eg.Go(
		func() (err error) {
			err = proxy.Connect(proxyContext, ports, app, false)
			return
		},
	)

	// do some other stuff
	closeProxy()
	eg.Wait()

	return
}
