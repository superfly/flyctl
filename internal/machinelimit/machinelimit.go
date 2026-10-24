// Package machinelimit tells customers when their organization is close to its
// Machines limit, and where to ask for more.
package machinelimit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// Advisory only, so never hold up the command for long.
var checkTimeout = 2 * time.Second

func RequestURL(orgSlug string) string {
	return fmt.Sprintf("https://fly.io/dashboard/%s/settings?request=machine-limit", orgSlug)
}

// Warn prints one line to stderr when the organization is using 80% or more of
// its Machines limit. Any failure stays silent.
func Warn(ctx context.Context, orgSlug string) {
	if config.FromContext(ctx).JSONOutput {
		return
	}

	quota, ok := machineQuota(ctx, orgSlug)
	if !ok {
		return
	}

	message := warning(orgSlug, quota)
	if message == "" {
		return
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintln(io.ErrOut)
	fmt.Fprintf(io.ErrOut, "%s %s\n", colorize.WarningIcon(), colorize.Yellow(message))
}

func warning(orgSlug string, quota uiex.MachineQuotaCheck) string {
	limit, ok := limitOf(quota)
	if !ok {
		return ""
	}

	request := "Request a higher limit at " + RequestURL(orgSlug)

	switch {
	case quota.Current >= limit:
		return fmt.Sprintf("Your organization has reached its limit of %d Machines. %s", limit, request)
	case quota.Current*5 >= limit*4:
		return fmt.Sprintf("Your organization is using %d of its %d Machines. %s", quota.Current, limit, request)
	default:
		return ""
	}
}

// Refused reports whether flaps turned down a Machine request. The Machine
// limit is one of the reasons it gives this status for.
func Refused(err error) bool {
	var flapsErr *flaps.FlapsError

	return errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusUnprocessableEntity
}

// WithRequestLink adds the request link when flaps refused a Machine and the
// organization is at its limit. It goes by the numbers rather than the
// wording of flaps' error, which carries no status code for this.
func WithRequestLink(ctx context.Context, err error, orgSlug string) error {
	if !Refused(err) {
		return err
	}

	quota, ok := machineQuota(ctx, orgSlug)
	if !ok {
		return err
	}

	if limit, ok := limitOf(quota); !ok || quota.Current < limit {
		return err
	}

	return flyerr.WithSuggestion(err, "Request a higher limit at "+RequestURL(orgSlug))
}

// AppOrgSlug finds the app's organization for callers that only know the app.
// It returns "" on any failure, which keeps Warn and WithRequestLink quiet.
func AppOrgSlug(ctx context.Context, appName string) string {
	client := flapsutil.ClientFromContext(ctx)
	if client == nil || appName == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return ""
	}

	return app.Organization.Slug
}

func machineQuota(ctx context.Context, orgSlug string) (uiex.MachineQuotaCheck, bool) {
	client := uiexutil.ClientFromContext(ctx)
	if client == nil || orgSlug == "" {
		return uiex.MachineQuotaCheck{}, false
	}

	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	checks, err := client.GetLaunchChecks(ctx, orgSlug)
	if err != nil {
		return uiex.MachineQuotaCheck{}, false
	}

	return checks.Checks.MachineQuota, true
}

// limitOf returns false when the org has no limit, or Machines are turned off.
func limitOf(quota uiex.MachineQuotaCheck) (int, bool) {
	if quota.Limit == nil || *quota.Limit <= 0 {
		return 0, false
	}

	return *quota.Limit, true
}
