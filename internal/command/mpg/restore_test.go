package mpg

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
)

func TestRunRestore(t *testing.T) {
	const (
		clusterID = "cluster-123"
		backupID  = "backup-456"
		pitrTime  = "2026-06-01T12:00:00Z"
	)

	tests := []struct {
		name            string
		clusterVersion  int
		backupID        string
		pitrTime        string
		wantErr         string
		wantErrContains string
		wantLookup      bool
		wantV1Restore   bool
		wantV2Restore   bool
	}{
		{
			name:           "requires a restore source",
			clusterVersion: 2,
			wantErr:        "one of --backup-id or --pitr-time is required",
		},
		{
			name:           "rejects both restore sources",
			clusterVersion: 2,
			backupID:       backupID,
			pitrTime:       pitrTime,
			wantErr:        "--backup-id and --pitr-time are mutually exclusive",
		},
		{
			name:           "rejects point in time restore for v1 before request",
			clusterVersion: 1,
			pitrTime:       pitrTime,
			wantErr:        "point-in-time restore is not supported for this cluster",
			wantLookup:     true,
		},
		{
			name:            "rejects malformed point in time before request",
			clusterVersion:  2,
			pitrTime:        "not-a-time",
			wantErrContains: "--pitr-time must be an RFC3339 timestamp with an explicit offset",
		},
		{
			name:            "rejects point in time without timezone before request",
			clusterVersion:  2,
			pitrTime:        "2026-06-01T12:00:00",
			wantErrContains: "--pitr-time must be an RFC3339 timestamp with an explicit offset",
		},
		{
			name:           "dispatches backup restore to v1",
			clusterVersion: 1,
			backupID:       backupID,
			wantLookup:     true,
			wantV1Restore:  true,
		},
		{
			name:           "dispatches backup restore to v2",
			clusterVersion: 2,
			backupID:       backupID,
			wantLookup:     true,
			wantV2Restore:  true,
		},
		{
			name:           "dispatches point in time restore to v2",
			clusterVersion: 2,
			pitrTime:       pitrTime,
			wantLookup:     true,
			wantV2Restore:  true,
		},
		{
			name:           "forwards point in time offset unchanged to v2",
			clusterVersion: 2,
			pitrTime:       "2026-06-01T14:00:00+02:00",
			wantLookup:     true,
			wantV2Restore:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext()
			flagSet := pflag.NewFlagSet("restore", pflag.ContinueOnError)
			flagSet.String("backup-id", tt.backupID, "")
			flagSet.String("name", "", "")
			flagSet.String("pitr-time", tt.pitrTime, "")
			require.NoError(t, flagSet.Parse([]string{clusterID}))
			ctx = flagctx.NewContext(ctx, flagSet)

			v1RestoreCalled := false
			v2RestoreCalled := false
			lookupCalled := false
			v1Client := &mock.MpgV1Client{
				GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
					lookupCalled = true
					if tt.clusterVersion != 1 {
						return mpgv1.GetManagedClusterResponse{}, errors.New("not found")
					}

					return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: clusterID}}, nil
				},
				RestoreManagedClusterBackupFunc: func(_ context.Context, gotClusterID string, input mpgv1.RestoreManagedClusterBackupInput) (mpgv1.RestoreManagedClusterBackupResponse, error) {
					v1RestoreCalled = true
					assert.Equal(t, clusterID, gotClusterID)
					assert.Equal(t, tt.backupID, input.BackupId)

					return mpgv1.RestoreManagedClusterBackupResponse{}, nil
				},
			}
			v2Client := &mock.MpgV2Client{
				RestoreClusterBackupFunc: func(_ context.Context, gotClusterID string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
					v2RestoreCalled = true
					assert.Equal(t, clusterID, gotClusterID)
					assert.Equal(t, tt.backupID, input.BackupId)
					assert.Equal(t, tt.pitrTime, input.PitrTime)

					return mpgv2.RestoreClusterBackupResponse{}, nil
				},
			}
			publicClient := &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					lookupCalled = true
					if tt.clusterVersion != 2 {
						return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
					}

					return flaps.ManagedPostgresCluster{ID: clusterID}, nil
				},
				RestoreManagedPostgresClusterFunc: func(_ context.Context, gotClusterID string, input flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					v2RestoreCalled = true
					assert.Equal(t, clusterID, gotClusterID)
					assert.Equal(t, tt.backupID, input.BackupID)
					assert.Equal(t, tt.pitrTime, input.PITRTime)

					return flaps.ManagedPostgresCluster{}, nil
				},
			}
			ctx = mpgv1.NewContextWithClient(ctx, v1Client)
			ctx = mpgv2.NewContextWithClient(ctx, v2Client)
			ctx = flapsutil.NewContextWithClient(ctx, publicClient)

			err := runRestore(ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else if tt.wantErrContains != "" {
				require.ErrorContains(t, err, tt.wantErrContains)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantLookup, lookupCalled)
			assert.Equal(t, tt.wantV1Restore, v1RestoreCalled)
			assert.Equal(t, tt.wantV2Restore, v2RestoreCalled)
		})
	}
}

func TestRunRestoreV2Serialization(t *testing.T) {
	const (
		clusterID = "cluster-123"
		backupID  = "backup-456"
		pitrTime  = "2026-06-01T12:00:00Z"
	)

	tests := []struct {
		name        string
		backupID    string
		restoreName string
		pitrTime    string
		wantFields  []string
		omitField   string
	}{
		{
			name:       "backup ID omits point in time",
			backupID:   backupID,
			wantFields: []string{`"backup_id"`},
			omitField:  `"pitr_time"`,
		},
		{
			name:       "point in time omits backup ID",
			pitrTime:   pitrTime,
			wantFields: []string{`"pitr_time"`},
			omitField:  `"backup_id"`,
		},
		{
			name:        "name and point in time coexist",
			restoreName: "restored-cluster",
			pitrTime:    pitrTime,
			wantFields:  []string{`"name"`, `"pitr_time"`},
			omitField:   `"backup_id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext()
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				RestoreManagedPostgresClusterFunc: func(context.Context, string, flaps.RestoreManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
				},
			})
			var capturedInput mpgv2.RestoreClusterBackupInput
			client := &mock.MpgV2Client{
				RestoreClusterBackupFunc: func(_ context.Context, gotClusterID string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
					assert.Equal(t, clusterID, gotClusterID)
					capturedInput = input

					return mpgv2.RestoreClusterBackupResponse{}, nil
				},
			}
			ctx = mpgv2.NewContextWithClient(ctx, client)

			require.NoError(t, cmdv2.RunRestore(ctx, clusterID, tt.backupID, tt.restoreName, tt.pitrTime))
			assert.Equal(t, tt.backupID, capturedInput.BackupId)
			assert.Equal(t, tt.restoreName, capturedInput.Name)
			assert.Equal(t, tt.pitrTime, capturedInput.PitrTime)

			body, err := json.Marshal(capturedInput)
			require.NoError(t, err)
			for _, wantField := range tt.wantFields {
				assert.Contains(t, string(body), wantField)
			}
			assert.NotContains(t, string(body), tt.omitField)
		})
	}
}
