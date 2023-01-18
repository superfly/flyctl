package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command/postgres"
)

func runOnDeletionHook(ctx context.Context, app *api.AppCompact, machine *api.Machine) {
	image := machine.ImageRef.Repository

	switch image {
	case "flyio/postgres-flex":
		if err := postgres.UnregisterMember(ctx, app, machine); err != nil {
			fmt.Printf("failed to unregister member: %v\n", err)
		}
	}
}
