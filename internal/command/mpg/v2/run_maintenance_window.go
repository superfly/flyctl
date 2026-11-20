package cmdv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// RunMaintenanceWindowShow prints the weekly maintenance window of clusterID.
func RunMaintenanceWindowShow(ctx context.Context, clusterID string) error {
	window, err := flapsutil.ClientFromContext(ctx).GetManagedPostgresMaintenanceWindow(ctx, clusterID)
	if err != nil {
		return maintenanceWindowError("get", clusterID, err)
	}

	return renderMaintenanceWindow(ctx, window)
}

// RunMaintenanceWindowSet changes the weekly maintenance window of clusterID
// and prints the window the API reports back. Fields left nil in req keep
// their current value.
func RunMaintenanceWindowSet(ctx context.Context, clusterID string, req flaps.UpdateManagedPostgresMaintenanceWindowRequest) error {
	window, err := flapsutil.ClientFromContext(ctx).UpdateManagedPostgresMaintenanceWindow(ctx, clusterID, req)
	if err != nil {
		return maintenanceWindowError("update", clusterID, err)
	}

	if !config.FromContext(ctx).JSONOutput {
		fmt.Fprintf(iostreams.FromContext(ctx).Out, "Updated maintenance window for cluster %s\n", clusterID)
	}

	return renderMaintenanceWindow(ctx, window)
}

func renderMaintenanceWindow(ctx context.Context, window flaps.ManagedPostgresMaintenanceWindow) error {
	out := iostreams.FromContext(ctx).Out

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, window)
	}

	rows := [][]string{{
		capitalize(window.Day),
		fmt.Sprintf("%02d:00", window.StartHour),
		formatHours(window.DurationHours),
		window.Timezone,
	}}

	return render.VerticalTable(out, "Maintenance Window", rows,
		"Day",
		"Start Time",
		"Duration",
		"Timezone",
	)
}

func formatHours(hours int) string {
	if hours == 1 {
		return "1 hour"
	}

	return strconv.Itoa(hours) + " hours"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}

	return strings.ToUpper(s[:1]) + s[1:]
}

// maintenanceWindowError turns the API's documented refusals into a one-line
// message carrying the API's own explanation, instead of fly-go's wrapped
// chain. The *flaps.FlapsError stays reachable through Unwrap so the request
// ID is still printed alongside the error.
func maintenanceWindowError(verb, clusterID string, err error) error {
	var flapsErr *flaps.FlapsError
	if errors.As(err, &flapsErr) {
		switch flapsErr.ResponseStatusCode {
		case http.StatusConflict, http.StatusUnprocessableEntity:
			return &apiRefusalError{
				msg: fmt.Sprintf("could not %s maintenance window for cluster %s: %s", verb, clusterID, apiErrorMessage(flapsErr)),
				err: flapsErr,
			}
		case http.StatusGone:
			return &apiRefusalError{
				msg: fmt.Sprintf("cluster %s has been deleted", clusterID),
				err: flapsErr,
			}
		}
	}

	return fmt.Errorf("failed to %s maintenance window for cluster %s: %w", verb, clusterID, err)
}

// apiErrorMessage returns the "error" field of the API's JSON error body,
// falling back to the HTTP status text when the body carries none.
func apiErrorMessage(err *flaps.FlapsError) string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(err.ResponseBody, &body) == nil {
		if msg := strings.TrimSpace(body.Error); msg != "" {
			return msg
		}
	}

	return strings.ToLower(http.StatusText(err.ResponseStatusCode))
}

type apiRefusalError struct {
	msg string
	err error
}

func (e *apiRefusalError) Error() string { return e.msg }

func (e *apiRefusalError) Unwrap() error { return e.err }
