package machine

import (
	"context"

	"github.com/samber/lo"
	"github.com/superfly/fly-go/api"
	"github.com/superfly/fly-go/flaps"
)

func ListActive(ctx context.Context) ([]*api.Machine, error) {
	flapsClient := flaps.FromContext(ctx)

	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return nil, err
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return m.Config != nil && m.IsActive() && !m.IsReleaseCommandMachine() && !m.IsFlyAppsConsole()
	})

	return machines, nil
}
