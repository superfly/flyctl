package launch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func ctxWithApp(app *flaps.App, err error) context.Context {
	return flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{
		GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
			return app, err
		},
	})
}

func TestNewFlyPostgresClusterParamsUsesAppNetwork(t *testing.T) {
	pgPlan := &plan.FlyPostgresPlan{AppName: "my-app-db", Nodes: 1}

	params, err := newFlyPostgresClusterParams(ctxWithApp(&flaps.App{Name: "my-app", Network: "custom"}, nil), pgPlan, "my-app")
	require.NoError(t, err)
	assert.Equal(t, "custom", params.Network)
	assert.Equal(t, "my-app-db", params.Name)
}

func TestNewFlyPostgresClusterParamsDefaultNetwork(t *testing.T) {
	pgPlan := &plan.FlyPostgresPlan{AppName: "my-app-db"}

	for _, network := range []string{"", "default"} {
		params, err := newFlyPostgresClusterParams(ctxWithApp(&flaps.App{Name: "my-app", Network: network}, nil), pgPlan, "my-app")
		require.NoError(t, err)
		assert.Empty(t, params.Network)
	}
}

func TestNewFlyPostgresClusterParamsAppLookupError(t *testing.T) {
	_, err := newFlyPostgresClusterParams(ctxWithApp(nil, errors.New("boom")), &plan.FlyPostgresPlan{}, "my-app")
	require.ErrorContains(t, err, "my-app")
}
