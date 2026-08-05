package mpg

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag/flagctx"
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
		name           string
		clusterVersion int
		backupID       string
		pitrTime       string
		wantErr        string
		wantV1Restore  bool
		wantV2Restore  bool
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
		},
		{
			name:           "dispatches backup restore to v1",
			clusterVersion: 1,
			backupID:       backupID,
			wantV1Restore:  true,
		},
		{
			name:           "dispatches backup restore to v2",
			clusterVersion: 2,
			backupID:       backupID,
			wantV2Restore:  true,
		},
		{
			name:           "dispatches point in time restore to v2",
			clusterVersion: 2,
			pitrTime:       pitrTime,
			wantV2Restore:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext()
			flagSet := pflag.NewFlagSet("restore", pflag.ContinueOnError)
			flagSet.String("backup-id", tt.backupID, "")
			flagSet.String("pitr-time", tt.pitrTime, "")
			require.NoError(t, flagSet.Parse([]string{clusterID}))
			ctx = flagctx.NewContext(ctx, flagSet)

			v1RestoreCalled := false
			v2RestoreCalled := false
			v1Client := &mock.MpgV1Client{
				GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
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
				GetClusterByIdFunc: func(context.Context, string) (mpgv2.GetClusterResponse, error) {
					if tt.clusterVersion != 2 {
						return mpgv2.GetClusterResponse{}, errors.New("not found")
					}
					return mpgv2.GetClusterResponse{Data: mpgv2.ManagedCluster{Id: clusterID, MpgdClusterId: "mpgd-123"}}, nil
				},
				RestoreClusterBackupFunc: func(_ context.Context, gotClusterID string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
					v2RestoreCalled = true
					assert.Equal(t, clusterID, gotClusterID)
					assert.Equal(t, tt.backupID, input.BackupId)
					assert.Equal(t, tt.pitrTime, input.PitrTime)
					return mpgv2.RestoreClusterBackupResponse{}, nil
				},
			}
			ctx = mpgv1.NewContextWithClient(ctx, v1Client)
			ctx = mpgv2.NewContextWithClient(ctx, v2Client)

			err := runRestore(ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
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
		name      string
		backupID  string
		pitrTime  string
		wantField string
		omitField string
	}{
		{
			name:      "backup ID omits point in time",
			backupID:  backupID,
			wantField: `"backup_id"`,
			omitField: `"pitr_time"`,
		},
		{
			name:      "point in time omits backup ID",
			pitrTime:  pitrTime,
			wantField: `"pitr_time"`,
			omitField: `"backup_id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext()
			var capturedInput mpgv2.RestoreClusterBackupInput
			client := &mock.MpgV2Client{
				RestoreClusterBackupFunc: func(_ context.Context, gotClusterID string, input mpgv2.RestoreClusterBackupInput) (mpgv2.RestoreClusterBackupResponse, error) {
					assert.Equal(t, clusterID, gotClusterID)
					capturedInput = input
					return mpgv2.RestoreClusterBackupResponse{}, nil
				},
			}
			ctx = mpgv2.NewContextWithClient(ctx, client)

			require.NoError(t, cmdv2.RunRestore(ctx, clusterID, tt.backupID, tt.pitrTime))
			assert.Equal(t, tt.backupID, capturedInput.BackupId)
			assert.Equal(t, tt.pitrTime, capturedInput.PitrTime)

			body, err := json.Marshal(capturedInput)
			require.NoError(t, err)
			assert.Contains(t, string(body), tt.wantField)
			assert.NotContains(t, string(body), tt.omitField)
		})
	}
}
