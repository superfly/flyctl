package machine

import (
	"context"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func runOnDeletionHook(ctx context.Context, appName string, machine *fly.Machine) {
	var (
		io     = iostreams.FromContext(ctx)
		labels = machine.ImageRef.Labels
	)

	if labels["fly.pg-manager"] == flypg.ReplicationManager {
		fmt.Fprintf(io.Out, "unregistering postgres member '%s' from the cluster... ", machine.PrivateIP)

		app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
		if err == nil {
			var org *uiex.Organization
			if org, err = uiexutil.AppOrganization(ctx, app); err == nil {
				err = postgres.UnregisterMember(ctx, app, org, machine)
			}
		}
		if err != nil {
			fmt.Fprintln(io.Out, "(failed)")
			fmt.Fprintf(io.Out, "failed to unregister postgres member: %v\n", err)

			return
		}
		fmt.Fprintln(io.Out, "(success)")
	}
}
