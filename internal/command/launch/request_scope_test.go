package launch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/iostreams"
)

func TestManagedPostgresScopeSurvivesCreation(t *testing.T) {
	assertOrg := authtest.Organization(t, 7)
	calls := 0
	client := &mock.FlapsClient{
		CreateManagedPostgresClusterFunc: func(ctx context.Context, _ flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			assertOrg(ctx)
			calls++
			return flaps.ManagedPostgresCluster{ID: "cluster"}, nil
		},
		GetManagedPostgresClusterFunc: func(ctx context.Context, _ string) (flaps.ManagedPostgresCluster, error) {
			assertOrg(ctx)
			calls++
			return flaps.ManagedPostgresCluster{}, errors.New("stop after checking poll credentials")
		},
	}
	ios, _, _, _ := iostreams.Test()
	ctx := flapsutil.NewContextWithClient(iostreams.NewContext(context.Background(), ios), client)
	state := &launchState{cache: map[string]any{"org,team": &uiex.Organization{Slug: "team", RawSlug: "team", InternalNumericID: 7}}}
	state.Plan = &plan.LaunchPlan{OrgSlug: "team"}
	state.Plan.Postgres.ManagedPostgres = &plan.ManagedPostgresPlan{DbName: "db"}
	err := state.createManagedPostgres(ctx)
	require.ErrorContains(t, err, "stop after checking poll credentials")
	require.Equal(t, 2, calls)
}
