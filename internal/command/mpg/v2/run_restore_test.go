package cmdv2

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func restoreTestContext() (context.Context, *bytes.Buffer) {
	io, _, stdout, _ := iostreams.Test()

	return iostreams.NewContext(context.Background(), io), stdout
}

func TestRunRestoreUsesPublicAPI(t *testing.T) {
	ctx, stdout := restoreTestContext()
	legacyCalled := false
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		RestoreManagedPostgresClusterFunc: func(_ context.Context, id string, req flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			require.Equal(t, "mpg-source", id)
			require.Equal(t, "backup-1", req.BackupID)
			require.Empty(t, req.PITRTime)
			require.Equal(t, "restored-db", req.Name)

			return flaps.ManagedPostgresCluster{ID: "mpg-restored", Name: "restored-db"}, nil
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		RestoreClusterBackupFunc: func(context.Context, string, mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
			legacyCalled = true

			return mpgv2.RestoreClusterBackupResponse{}, nil
		},
	})

	require.NoError(t, RunRestore(ctx, "mpg-source", "backup-1", "restored-db", ""))
	require.False(t, legacyCalled)
	require.Contains(t, stdout.String(), "Cluster ID: mpg-restored")
	require.Contains(t, stdout.String(), "Cluster Name: restored-db")
}

func TestRunRestoreFallsBackOnPublic404(t *testing.T) {
	ctx, stdout := restoreTestContext()
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		RestoreManagedPostgresClusterFunc: func(context.Context, string, flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		RestoreClusterBackupFunc: func(_ context.Context, id string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
			require.Equal(t, "mpg-source", id)
			require.Empty(t, input.BackupId)
			require.Equal(t, "2026-06-01T12:02:30Z", input.PitrTime)
			require.Equal(t, "restored-db", input.Name)

			return mpgv2.RestoreClusterBackupResponse{Data: mpgv2.ManagedCluster{Id: "legacy-restored", Name: "restored-db"}}, nil
		},
	})

	require.NoError(t, RunRestore(ctx, "mpg-source", "", "restored-db", "2026-06-01T12:02:30Z"))
	require.Contains(t, stdout.String(), "Cluster ID: legacy-restored")
}

func TestRunRestoreDoesNotFallbackOnOtherPublicErrors(t *testing.T) {
	ctx, _ := restoreTestContext()
	publicErr := &flaps.FlapsError{ResponseStatusCode: 422, OriginalError: errors.New("outside recovery window")}
	legacyCalled := false
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		RestoreManagedPostgresClusterFunc: func(context.Context, string, flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, publicErr
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		RestoreClusterBackupFunc: func(context.Context, string, mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
			legacyCalled = true

			return mpgv2.RestoreClusterBackupResponse{}, nil
		},
	})

	err := RunRestore(ctx, "mpg-source", "", "", "2026-06-01T12:02:30Z")
	require.ErrorIs(t, err, publicErr)
	require.ErrorContains(t, err, "failed to restore cluster: outside recovery window")
	require.False(t, legacyCalled)
}

func TestRunRestorePreservesPublic404WhenFallbackFails(t *testing.T) {
	ctx, _ := restoreTestContext()
	publicErr := &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("backup not found")}
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		RestoreManagedPostgresClusterFunc: func(context.Context, string, flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, publicErr
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		RestoreClusterBackupFunc: func(context.Context, string, mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
			return mpgv2.RestoreClusterBackupResponse{}, errors.New("legacy restore failed")
		},
	})

	err := RunRestore(ctx, "mpg-source", "backup-1", "", "")
	require.ErrorIs(t, err, publicErr)
	require.EqualError(t, err, "failed to restore cluster: backup not found")
}
