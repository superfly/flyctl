package mpg

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func TestRunFork(t *testing.T) {
	const clusterID = "cluster-123"

	tests := []struct {
		name           string
		clusterVersion int
		args           []string
		wantErr        string
		wantFork       bool
		wantName       string
		wantPlan       string
	}{
		{
			name:           "rejects v1 cluster before request",
			clusterVersion: 1,
			args:           []string{clusterID, "--plan", "basic"},
			wantErr:        "forking is not supported for this cluster",
		},
		{
			name:           "sends no plan when --plan is omitted",
			clusterVersion: 2,
			args:           []string{clusterID},
			wantFork:       true,
		},
		{
			name:           "passes name and plan through",
			clusterVersion: 2,
			args:           []string{clusterID, "--name", "forked-db", "--plan", "starter"},
			wantFork:       true,
			wantName:       "forked-db",
			wantPlan:       "starter",
		},
		{
			name:           "accepts -n shorthand for name",
			clusterVersion: 2,
			args:           []string{clusterID, "-n", "forked-db"},
			wantFork:       true,
			wantName:       "forked-db",
		},
		{
			name:           "trims plan whitespace",
			clusterVersion: 2,
			args:           []string{clusterID, "--plan", "  launch  "},
			wantFork:       true,
			wantPlan:       "launch",
		},
		{
			name:           "blank plan keeps the source plan",
			clusterVersion: 2,
			args:           []string{clusterID, "--plan", "   "},
			wantFork:       true,
		},
		{
			name:           "does not validate plan against the local plan list",
			clusterVersion: 2,
			args:           []string{clusterID, "--plan", "PERFORMANCE"},
			wantFork:       true,
			wantPlan:       "PERFORMANCE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext()
			cmd := newFork()
			require.NoError(t, cmd.Flags().Parse(tt.args))
			ctx = flagctx.NewContext(ctx, cmd.Flags())

			forkCalled := false
			v1Client := &mock.MpgV1Client{
				GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
					if tt.clusterVersion != 1 {
						return mpgv1.GetManagedClusterResponse{}, errors.New("not found")
					}

					return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: clusterID}}, nil
				},
			}
			publicClient := &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					if tt.clusterVersion != 2 {
						return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
					}

					return flaps.ManagedPostgresCluster{ID: clusterID, Name: "source-db"}, nil
				},
				ForkManagedPostgresClusterFunc: func(_ context.Context, gotClusterID string, req flaps.ForkManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					forkCalled = true
					assert.Equal(t, clusterID, gotClusterID)
					assert.Equal(t, tt.wantName, req.Name)
					assert.Equal(t, tt.wantPlan, req.Plan)

					return flaps.ManagedPostgresCluster{ID: "cluster-fork", Name: "source-db-fork", Plan: "basic"}, nil
				},
			}
			ctx = mpgv1.NewContextWithClient(ctx, v1Client)
			ctx = flapsutil.NewContextWithClient(ctx, publicClient)

			err := runFork(ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantFork, forkCalled)
		})
	}
}

func TestNewForkFlags(t *testing.T) {
	cmd := newFork()

	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
	require.NoError(t, cmd.Args(cmd, []string{}))
	require.NoError(t, cmd.Args(cmd, []string{"a"}))

	name := cmd.Flags().Lookup("name")
	require.NotNil(t, name)
	assert.Equal(t, "n", name.Shorthand)
	assert.Empty(t, name.DefValue)

	plan := cmd.Flags().Lookup("plan")
	require.NotNil(t, plan)
	assert.Empty(t, plan.DefValue)
	assert.Contains(t, plan.Usage, planNames())
	assert.Contains(t, plan.Usage, "defaults to the source cluster's plan")
}
