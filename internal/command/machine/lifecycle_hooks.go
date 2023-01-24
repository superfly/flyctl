package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/iostreams"
)

func runOnDeletionHook(ctx context.Context, app *api.AppCompact, machine *api.Machine) {
	io := iostreams.FromContext(ctx)

	image := machine.ImageRef.Repository

	switch image {
	case "flyio/postgres-flex":
		fmt.Fprintf(io.Out, "unregistering postgres member '%s' from the cluster... ", machine.PrivateIP)
		if err := postgres.UnregisterMember(ctx, app, machine); err != nil {
			fmt.Fprintln(io.Out, "(failed)")
			fmt.Fprintf(io.Out, "failed to unregister postgres member: %v\n", err)
		}
		fmt.Fprintln(io.Out, "(success)")
	}
}
