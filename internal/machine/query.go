package machine

import (
	"context"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
)

func ListActive(ctx context.Context) ([]*api.Machine, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
	)

	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return nil, err
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return m.Config != nil && m.Config.Metadata["process_group"] != "release_command" && m.State != "destroyed"
	})

	return machines, nil

}
