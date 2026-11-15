package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func maintenanceWindowTestContext(t *testing.T, jsonOutput bool, client *mock.FlapsClient) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})

	return flapsutil.NewContextWithClient(ctx, client), stdout
}

func sampleMaintenanceWindow() flaps.ManagedPostgresMaintenanceWindow {
	return flaps.ManagedPostgresMaintenanceWindow{
		Day:           "sunday",
		StartHour:     1,
		DurationHours: 3,
		Timezone:      "America/New_York",
	}
}

func requireMaintenanceWindowTable(t *testing.T, output string, day, start, duration, timezone string) {
	t.Helper()

	require.Contains(t, output, "Maintenance Window")
	require.Regexp(t, columnLineRegex("Day", day), output)
	require.Regexp(t, columnLineRegex("Start Time", start), output)
	require.Regexp(t, columnLineRegex("Duration", duration), output)
	require.Regexp(t, columnLineRegex("Timezone", timezone), output)
}

func TestRunMaintenanceWindowShowHuman(t *testing.T) {
	ctx, stdout := maintenanceWindowTestContext(t, false, &mock.FlapsClient{
		GetManagedPostgresMaintenanceWindowFunc: func(_ context.Context, id string) (flaps.ManagedPostgresMaintenanceWindow, error) {
			require.Equal(t, "mpg-123", id)

			return sampleMaintenanceWindow(), nil
		},
	})

	require.NoError(t, RunMaintenanceWindowShow(ctx, "mpg-123"))
	requireMaintenanceWindowTable(t, stdout.String(), "Sunday", "01:00", "3 hours", "America/New_York")
}

func TestRunMaintenanceWindowShowJSON(t *testing.T) {
	ctx, stdout := maintenanceWindowTestContext(t, true, &mock.FlapsClient{
		GetManagedPostgresMaintenanceWindowFunc: func(context.Context, string) (flaps.ManagedPostgresMaintenanceWindow, error) {
			return sampleMaintenanceWindow(), nil
		},
	})

	require.NoError(t, RunMaintenanceWindowShow(ctx, "mpg-123"))
	var got map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	require.Equal(t, map[string]any{
		"day":            "sunday",
		"start_hour":     float64(1),
		"duration_hours": float64(3),
		"timezone":       "America/New_York",
	}, got)
}

func TestRunMaintenanceWindowSetSendsRequestAndPrintsUpdatedWindow(t *testing.T) {
	day := "saturday"
	hour := 0
	req := flaps.UpdateManagedPostgresMaintenanceWindowRequest{Day: &day, StartHour: &hour}

	ctx, stdout := maintenanceWindowTestContext(t, false, &mock.FlapsClient{
		UpdateManagedPostgresMaintenanceWindowFunc: func(_ context.Context, id string, got flaps.UpdateManagedPostgresMaintenanceWindowRequest) (flaps.ManagedPostgresMaintenanceWindow, error) {
			require.Equal(t, "mpg-123", id)
			require.Equal(t, req, got)

			return flaps.ManagedPostgresMaintenanceWindow{Day: "saturday", StartHour: 0, DurationHours: 4, Timezone: "Europe/Paris"}, nil
		},
	})

	require.NoError(t, RunMaintenanceWindowSet(ctx, "mpg-123", req))
	output := stdout.String()
	require.Contains(t, output, "Updated maintenance window for cluster mpg-123\n")
	requireMaintenanceWindowTable(t, output, "Saturday", "00:00", "4 hours", "Europe/Paris")
}

func TestRunMaintenanceWindowSetJSONPrintsOnlyTheWindow(t *testing.T) {
	duration := 8
	ctx, stdout := maintenanceWindowTestContext(t, true, &mock.FlapsClient{
		UpdateManagedPostgresMaintenanceWindowFunc: func(context.Context, string, flaps.UpdateManagedPostgresMaintenanceWindowRequest) (flaps.ManagedPostgresMaintenanceWindow, error) {
			return flaps.ManagedPostgresMaintenanceWindow{Day: "monday", StartHour: 23, DurationHours: 8, Timezone: "UTC"}, nil
		},
	})

	require.NoError(t, RunMaintenanceWindowSet(ctx, "mpg-123", flaps.UpdateManagedPostgresMaintenanceWindowRequest{DurationHours: &duration}))
	var got flaps.ManagedPostgresMaintenanceWindow
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got), "stdout must be pure JSON")
	require.Equal(t, flaps.ManagedPostgresMaintenanceWindow{Day: "monday", StartHour: 23, DurationHours: 8, Timezone: "UTC"}, got)
}

// apiError builds the error fly-go returns for a non-2xx response, including
// its own wrapping.
func apiError(verb string, status int, body string) error {
	return fmt.Errorf("failed to %s Managed Postgres maintenance window: %w", verb, &flaps.FlapsError{
		OriginalError:      errors.New("raw " + body),
		ResponseStatusCode: status,
		ResponseBody:       []byte(body),
		FlyRequestId:       "req-123",
	})
}

func TestMaintenanceWindowErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{
			name:    "409 surfaces the API's message",
			err:     apiError("update", 409, `{"error":"maintenance is currently running on this cluster"}`),
			wantErr: "could not update maintenance window for cluster mpg-123: maintenance is currently running on this cluster",
		},
		{
			name:    "422 surfaces the API's message",
			err:     apiError("update", 422, `{"error":"duration_hours must be between 3 and 8"}`),
			wantErr: "could not update maintenance window for cluster mpg-123: duration_hours must be between 3 and 8",
		},
		{
			name:    "422 without a message falls back to the status text",
			err:     apiError("update", 422, `not json`),
			wantErr: "could not update maintenance window for cluster mpg-123: unprocessable entity",
		},
		{
			name:    "410 reports the cluster as deleted",
			err:     apiError("update", 410, `{"error":"gone"}`),
			wantErr: "cluster mpg-123 has been deleted",
		},
		{
			name:    "other API errors keep the full chain",
			err:     apiError("update", 500, `{"error":"boom"}`),
			wantErr: "failed to update maintenance window for cluster mpg-123: failed to update Managed Postgres maintenance window: raw {\"error\":\"boom\"}",
		},
		{
			name:    "transport errors keep the full chain",
			err:     errors.New("connection refused"),
			wantErr: "failed to update maintenance window for cluster mpg-123: connection refused",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := maintenanceWindowTestContext(t, false, &mock.FlapsClient{
				UpdateManagedPostgresMaintenanceWindowFunc: func(context.Context, string, flaps.UpdateManagedPostgresMaintenanceWindowRequest) (flaps.ManagedPostgresMaintenanceWindow, error) {
					return flaps.ManagedPostgresMaintenanceWindow{}, test.err
				},
			})

			hour := 2
			err := RunMaintenanceWindowSet(ctx, "mpg-123", flaps.UpdateManagedPostgresMaintenanceWindowRequest{StartHour: &hour})
			require.EqualError(t, err, test.wantErr)
			require.Empty(t, stdout.String(), "no success output on error")

			var flapsErr *flaps.FlapsError
			if errors.As(test.err, &flapsErr) {
				require.Equal(t, "req-123", flaps.GetErrorRequestID(err), "request ID must stay reachable for the error printer")
			}
		})
	}
}

func TestRunMaintenanceWindowShowErrors(t *testing.T) {
	ctx, stdout := maintenanceWindowTestContext(t, false, &mock.FlapsClient{
		GetManagedPostgresMaintenanceWindowFunc: func(context.Context, string) (flaps.ManagedPostgresMaintenanceWindow, error) {
			return flaps.ManagedPostgresMaintenanceWindow{}, apiError("get", 409, `{"error":"cluster is not ready"}`)
		},
	})

	require.EqualError(t, RunMaintenanceWindowShow(ctx, "mpg-123"), "could not get maintenance window for cluster mpg-123: cluster is not ready")
	require.Empty(t, stdout.String())
}
