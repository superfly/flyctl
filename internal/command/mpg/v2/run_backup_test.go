package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func backupTestContext(t *testing.T, jsonOutput, showAll bool) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("all", showAll, "")
	flags.String("type", "", "")

	return flagctx.NewContext(ctx, flags), stdout
}

func TestRunBackupListUsesPublicAPIAndLegacyJSONShape(t *testing.T) {
	ctx, stdout := backupTestContext(t, true, true)
	public := []flaps.ManagedPostgresBackup{{
		ID: "backup-1", Status: "completed", Type: "full", SizeBytes: 42,
		StartedAt: "2026-08-26T12:00:00Z", FinishedAt: "2026-08-26T12:05:00Z",
	}}
	publicCalled := false
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresBackupsFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresBackup, error) {
			publicCalled = true
			require.Equal(t, "mpg-123", id)

			return public, nil
		},
	})
	require.NoError(t, RunBackupList(ctx, "mpg-123"))
	require.True(t, publicCalled)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	require.Equal(t, []map[string]any{{
		"id": "backup-1", "status": "completed", "type": "full",
		"start": "2026-08-26T12:00:00Z", "stop": "2026-08-26T12:05:00Z",
	}}, got)
}

func TestRunBackupListFiltersPublicStartTimestamp(t *testing.T) {
	ctx, stdout := backupTestContext(t, false, false)
	public := []flaps.ManagedPostgresBackup{
		{ID: "old", StartedAt: time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339), Type: "full"},
		{ID: "new", StartedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), Type: "incr"},
	}
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresBackupsFunc: func(context.Context, string) ([]flaps.ManagedPostgresBackup, error) {
			return public, nil
		},
	})
	require.NoError(t, RunBackupList(ctx, "mpg-123"))
	require.Contains(t, stdout.String(), "new")
	require.NotContains(t, stdout.String(), "old")
}

func TestRunBackupListEmptyPublicResult(t *testing.T) {
	ctx, stdout := backupTestContext(t, false, true)
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresBackupsFunc: func(context.Context, string) ([]flaps.ManagedPostgresBackup, error) {
			return []flaps.ManagedPostgresBackup{}, nil
		},
	})
	require.NoError(t, RunBackupList(ctx, "mpg-123"))
	require.Contains(t, stdout.String(), "No backups found for cluster mpg-123")
}

func TestRunBackupListReturnsPublicError(t *testing.T) {
	tests := []struct {
		name             string
		publicErr        error
		wantErr          string
		wantPublicStatus int
	}{
		{name: "non-404", publicErr: &flaps.FlapsError{ResponseStatusCode: 500, OriginalError: errors.New("public boom")}, wantErr: "failed to list backups for cluster mpg-123: public boom", wantPublicStatus: 500},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := backupTestContext(t, false, true)
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListManagedPostgresBackupsFunc: func(context.Context, string) ([]flaps.ManagedPostgresBackup, error) {
					return nil, test.publicErr
				},
			})
			err := RunBackupList(ctx, "mpg-123")
			if test.wantPublicStatus != 0 {
				var publicErr *flaps.FlapsError
				require.True(t, errors.As(err, &publicErr))
				require.Equal(t, test.wantPublicStatus, publicErr.ResponseStatusCode)
			}
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantErr)
			}
		})
	}
}

func TestRunBackupListReturnsPublic404(t *testing.T) {
	ctx, _ := backupTestContext(t, false, true)
	publicErr := &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("cluster not found")}
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresBackupsFunc: func(context.Context, string) ([]flaps.ManagedPostgresBackup, error) {
			return nil, publicErr
		},
	})
	err := RunBackupList(ctx, "mpg-123")
	require.ErrorIs(t, err, publicErr)
	require.ErrorContains(t, err, "failed to list backups for cluster mpg-123: cluster not found")
}

func TestRunBackupCreateUsesPublicAPI(t *testing.T) {
	tests := []struct {
		name             string
		publicErr        error
		wantErr          string
		wantPublicStatus int
	}{
		{name: "public", wantErr: ""},
		{name: "non-404", publicErr: errors.New("public boom"), wantErr: "failed to create backup: public boom"},
		{name: "concurrent backup 409", publicErr: &flaps.FlapsError{ResponseStatusCode: 409, OriginalError: errors.New("backup already in progress")}, wantErr: "failed to create backup: backup already in progress", wantPublicStatus: 409},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := backupTestContext(t, false, false)
			require.NoError(t, flag.SetString(ctx, "type", "incr"))
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				CreateManagedPostgresBackupFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresBackupRequest) error {
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "incr", req.Type)

					return test.publicErr
				},
			})
			err := RunBackupCreate(ctx, "mpg-123")
			if test.wantPublicStatus != 0 {
				var publicErr *flaps.FlapsError
				require.True(t, errors.As(err, &publicErr))
				require.Equal(t, test.wantPublicStatus, publicErr.ResponseStatusCode)
			}
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantErr)
			}
		})
	}
}

func TestRunBackupCreateRejectsOtherTypes(t *testing.T) {
	ctx, _ := backupTestContext(t, false, false)
	require.NoError(t, flag.SetString(ctx, "type", "diff"))
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{})
	require.EqualError(t, RunBackupCreate(ctx, "mpg-123"), "--type must be either 'full' or 'incr'")
}
