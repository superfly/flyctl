package machine

import (
	"context"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func runOnDeletionHook(ctx context.Context, appName string, machine *fly.Machine) {
	var (
		io     = iostreams.FromContext(ctx)
		labels = machine.ImageRef.Labels
	)

	if labels["fly.pg-manager"] == flypg.ReplicationManager {
		fmt.Fprintf(io.Out, "unregistering postgres member '%s' from the cluster... ", machine.PrivateIP)

		apiClient := flyutil.ClientFromContext(ctx)
		appCompact, err := apiClient.GetAppCompact(ctx, appName)
		if err != nil {
			fmt.Fprintln(io.Out, "(failed)")
			fmt.Fprintf(io.Out, "failed to unregister postgres member: %v\n", err)
			return
		}

		if err := postgres.UnregisterMember(ctx, appCompact, machine); err != nil {
			fmt.Fprintln(io.Out, "(failed)")
			fmt.Fprintf(io.Out, "failed to unregister postgres member: %v\n", err)
			return
		}
		fmt.Fprintln(io.Out, "(success)")
	}
}
