package mpg

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func ptr[T any](v T) *T { return &v }

// maintenanceWindowCommandContext wires a v1 or v2 cluster lookup and records
// the maintenance window calls the command makes.
func maintenanceWindowCommandContext(t *testing.T, cmd *cobra.Command, clusterVersion int, client *mock.FlapsClient, args ...string) (context.Context, error) {
	t.Helper()

	const clusterID = "cluster-123"

	ctx := setupTestContext()
	ctx = config.NewContext(ctx, &config.Config{})

	if err := cmd.Flags().Parse(args); err != nil {
		return nil, err
	}
	ctx = flagctx.NewContext(ctx, cmd.Flags())

	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
			if clusterVersion != 1 {
				return mpgv1.GetManagedClusterResponse{}, errors.New("not found")
			}

			return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{Id: clusterID}}, nil
		},
	})
	client.GetManagedPostgresClusterFunc = func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
		if clusterVersion != 2 {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		}
		assert.Equal(t, clusterID, id)

		return flaps.ManagedPostgresCluster{ID: clusterID, Name: "db"}, nil
	}

	return flapsutil.NewContextWithClient(ctx, client), nil
}

func TestRunMaintenanceWindowSet(t *testing.T) {
	tests := []struct {
		name           string
		clusterVersion int
		args           []string
		wantErr        string
		wantRequest    *flaps.UpdateManagedPostgresMaintenanceWindowRequest
	}{
		{
			name:           "sends every flag",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--day", "tuesday", "--start-hour", "4", "--duration", "5"},
			wantRequest: &flaps.UpdateManagedPostgresMaintenanceWindowRequest{
				Day: ptr("tuesday"), StartHour: ptr(4), DurationHours: ptr(5),
			},
		},
		{
			name:           "sends only the flags that were set",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--duration", "6"},
			wantRequest:    &flaps.UpdateManagedPostgresMaintenanceWindowRequest{DurationHours: ptr(6)},
		},
		{
			name:           "sends --start-hour 0",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--start-hour", "0"},
			wantRequest:    &flaps.UpdateManagedPostgresMaintenanceWindowRequest{StartHour: ptr(0)},
		},
		{
			name:           "lowercases and trims the day",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--day", " SunDay "},
			wantRequest:    &flaps.UpdateManagedPostgresMaintenanceWindowRequest{Day: ptr("sunday")},
		},
		{
			name:           "leaves duration bounds to the API",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--duration", "12"},
			wantRequest:    &flaps.UpdateManagedPostgresMaintenanceWindowRequest{DurationHours: ptr(12)},
		},
		{
			name:           "requires at least one flag",
			clusterVersion: 2,
			args:           []string{"cluster-123"},
			wantErr:        "specify at least one of --day, --start-hour or --duration",
		},
		{
			name:           "rejects an unknown day",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--day", "funday"},
			wantErr:        `--day must be a day of the week (monday, tuesday, wednesday, thursday, friday, saturday, sunday), got "funday"`,
		},
		{
			name:           "rejects an hour past 23",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--start-hour", "24"},
			wantErr:        "--start-hour must be between 0 and 23, got 24",
		},
		{
			name:           "rejects a negative hour",
			clusterVersion: 2,
			args:           []string{"cluster-123", "--start-hour=-1"},
			wantErr:        "--start-hour must be between 0 and 23, got -1",
		},
		{
			name:           "rejects v1 clusters before the request",
			clusterVersion: 1,
			args:           []string{"cluster-123", "--day", "monday"},
			wantErr:        "maintenance windows are not supported for this cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRequest *flaps.UpdateManagedPostgresMaintenanceWindowRequest
			client := &mock.FlapsClient{
				UpdateManagedPostgresMaintenanceWindowFunc: func(_ context.Context, id string, req flaps.UpdateManagedPostgresMaintenanceWindowRequest) (flaps.ManagedPostgresMaintenanceWindow, error) {
					assert.Equal(t, "cluster-123", id)
					gotRequest = &req

					return flaps.ManagedPostgresMaintenanceWindow{Day: "sunday", StartHour: 0, DurationHours: 3, Timezone: "UTC"}, nil
				},
			}
			ctx, err := maintenanceWindowCommandContext(t, newMaintenanceWindowSet(), tt.clusterVersion, client, tt.args...)
			require.NoError(t, err)

			err = runMaintenanceWindowSet(ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequest, gotRequest)
		})
	}
}

func TestRunMaintenanceWindowShow(t *testing.T) {
	tests := []struct {
		name           string
		clusterVersion int
		wantErr        string
		wantGet        bool
	}{
		{name: "fetches the window for a v2 cluster", clusterVersion: 2, wantGet: true},
		{name: "rejects v1 clusters", clusterVersion: 1, wantErr: "maintenance windows are not supported for this cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getCalled := false
			client := &mock.FlapsClient{
				GetManagedPostgresMaintenanceWindowFunc: func(_ context.Context, id string) (flaps.ManagedPostgresMaintenanceWindow, error) {
					getCalled = true
					assert.Equal(t, "cluster-123", id)

					return flaps.ManagedPostgresMaintenanceWindow{Day: "sunday", StartHour: 1, DurationHours: 3, Timezone: "UTC"}, nil
				},
			}
			ctx, err := maintenanceWindowCommandContext(t, newMaintenanceWindowShow(), tt.clusterVersion, client, "cluster-123")
			require.NoError(t, err)

			err = runMaintenanceWindowShow(ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantGet, getCalled)
		})
	}
}

func TestMaintenanceWindowCommands(t *testing.T) {
	group, _, err := New().Find([]string{"maintenance-window"})
	require.NoError(t, err)
	assert.Equal(t, "maintenance-window", group.Name())
	assert.Contains(t, group.Aliases, "maintenance")

	for _, name := range []string{"show", "set"} {
		sub, _, err := group.Find([]string{name})
		require.NoError(t, err)
		assert.Equal(t, name, sub.Name())
		require.Error(t, sub.Args(sub, []string{"a", "b"}))
		require.NoError(t, sub.Args(sub, []string{}))
		assert.NotNil(t, sub.Flags().Lookup("json"))
	}

	set := newMaintenanceWindowSet()
	for _, name := range []string{"day", "start-hour", "duration"} {
		assert.NotNil(t, set.Flags().Lookup(name), name)
	}
}
