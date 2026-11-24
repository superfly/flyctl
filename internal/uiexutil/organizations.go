package uiexutil

import (
	"context"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/requestauth"
	"github.com/superfly/flyctl/internal/uiex"
)

// AppOrganization returns the organization that owns app, looked up by the
// org slug Flaps reports for it.
func AppOrganization(ctx context.Context, app *flaps.App) (*uiex.Organization, error) {
	ctx = requestauth.WithOrganizations(ctx, uint64(app.Organization.InternalNumericID))
	org, err := ClientFromContext(ctx).GetOrganization(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving organization for app %s: %w", app.Name, err)
	}

	return org, nil
}
