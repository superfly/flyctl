package mpg

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunUsersRotatePasswordJSONRequiresClusterBeforePicker(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdinTTY(true)
	io.SetStdoutTTY(true)
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	ctx = flagctx.NewContext(ctx, flags)
	// No client context: reaching lookup or selection before validating the
	// arguments would panic, as well as potentially corrupting JSON stdout.
	err := runUsersRotatePassword(ctx)
	require.ErrorContains(t, err, "CLUSTER_ID must be specified when using --json")
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}

// v1ClusterContext parses args for cmd and serves "mpg-v1" as a v1 cluster.
// The v1 client has no user functions, so reaching them would panic.
func v1ClusterContext(t *testing.T, cmd *cobra.Command, args ...string) context.Context {
	t.Helper()
	require.NoError(t, cmd.Flags().Parse(args))
	ctx := flagctx.NewContext(setupTestContext(), cmd.Flags())
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})

	return mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(_ context.Context, id string) (mpgv1.GetManagedClusterResponse, error) {
			if id != "mpg-v1" {
				return mpgv1.GetManagedClusterResponse{}, errors.New("not found")
			}

			return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: id, Version: 1}}, nil
		},
	})
}

func TestUsersDatabaseLimitsRefusedOnV1(t *testing.T) {
	ctx := v1ClusterContext(t, newUsersCreate(), "mpg-v1", "-u", "analyst", "-r", "custom", "-d", "app1")
	require.EqualError(t, runUsersCreate(ctx), "--database is not supported for v1 clusters; migrate the cluster to v2 first")

	ctx = v1ClusterContext(t, newUsersSetDatabases(), "mpg-v1", "-u", "analyst", "--all-databases")
	require.EqualError(t, runUsersSetDatabases(ctx), "'fly mpg users set-databases' is not supported for v1 clusters; migrate the cluster to v2 first")
}

func TestRolesRefusedOnV1(t *testing.T) {
	ctx := v1ClusterContext(t, newRolesList(), "mpg-v1")
	require.EqualError(t, runRolesList(ctx), "'fly mpg roles list' is not supported for v1 clusters; migrate the cluster to v2 first")

	ctx = v1ClusterContext(t, newRolesMembers(), "mpg-v1", "-r", "analytics")
	require.EqualError(t, runRolesMembers(ctx), "'fly mpg roles members' is not supported for v1 clusters; migrate the cluster to v2 first")
}
