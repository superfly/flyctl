package machine

import (
	"context"
	"time"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func ListActive(ctx context.Context) ([]*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := RetryRet(0, func(_ time.Duration) ([]*fly.Machine, error) {
		return flapsClient.List(ctx, "")
	})
	if err != nil {
		return nil, err
	}

	machines = lo.Filter(machines, func(m *fly.Machine, _ int) bool {
		return m.Config != nil && m.IsActive() && !m.IsReleaseCommandMachine() && !m.IsFlyAppsConsole()
	})

	return machines, nil
}
