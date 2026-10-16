package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func forkTestContext() (context.Context, *bytes.Buffer) {
	io, _, stdout, _ := iostreams.Test()

	return iostreams.NewContext(context.Background(), io), stdout
}

func TestRunForkPassesNameAndPlan(t *testing.T) {
	ctx, stdout := forkTestContext()
	called := false
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ForkManagedPostgresClusterFunc: func(_ context.Context, id string, req flaps.ForkManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			called = true
			require.Equal(t, "mpg-source", id)
			require.Equal(t, "forked-db", req.Name)
			require.Equal(t, "starter", req.Plan)

			return flaps.ManagedPostgresCluster{ID: "mpg-fork", Name: "forked-db", Plan: "starter"}, nil
		},
	})

	require.NoError(t, RunFork(ctx, "mpg-source", "forked-db", "starter"))
	require.True(t, called)
	require.Contains(t, stdout.String(), "Forking cluster mpg-source...")
	require.Contains(t, stdout.String(), "Fork initiated successfully!")
	require.Contains(t, stdout.String(), "Cluster ID: mpg-fork")
	require.Contains(t, stdout.String(), "Cluster Name: forked-db")
	require.Contains(t, stdout.String(), "Plan: starter")
}

func TestRunForkWithoutPlanOmitsPlan(t *testing.T) {
	ctx, stdout := forkTestContext()
	var captured flaps.ForkManagedPostgresClusterRequest
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ForkManagedPostgresClusterFunc: func(_ context.Context, _ string, req flaps.ForkManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			captured = req

			// The API copies the source cluster's plan when none is given.
			return flaps.ManagedPostgresCluster{ID: "mpg-fork", Name: "source-db-fork", Plan: "launch"}, nil
		},
	})

	require.NoError(t, RunFork(ctx, "mpg-source", "", ""))
	require.Empty(t, captured.Plan)
	require.Empty(t, captured.Name)

	body, err := json.Marshal(captured)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(body))

	require.Contains(t, stdout.String(), "Cluster ID: mpg-fork")
	require.Contains(t, stdout.String(), "Cluster Name: source-db-fork")
	require.Contains(t, stdout.String(), "Plan: launch")
}

func TestRunForkWrapsError(t *testing.T) {
	ctx, stdout := forkTestContext()
	publicErr := &flaps.FlapsError{ResponseStatusCode: 422, OriginalError: errors.New("plan is invalid")}
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ForkManagedPostgresClusterFunc: func(context.Context, string, flaps.ForkManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, publicErr
		},
	})

	err := RunFork(ctx, "mpg-source", "", "nope")
	require.ErrorIs(t, err, publicErr)
	require.EqualError(t, err, "failed to fork cluster: plan is invalid")
	require.NotContains(t, stdout.String(), "Fork initiated successfully!")
}

func TestRunForkDoesNotFallBackOnPublic404(t *testing.T) {
	ctx, _ := forkTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ForkManagedPostgresClusterFunc: func(context.Context, string, flaps.ForkManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})

	err := RunFork(ctx, "mpg-missing", "", "")
	require.ErrorIs(t, err, flaps.ErrFlapsNotFound)
	require.ErrorContains(t, err, "failed to fork cluster:")
}
