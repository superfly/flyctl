package mpg

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
	"github.com/superfly/flyctl/internal/uiex"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/internal/uiexutil"
)

func TestAttachmentOrgScope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		run      func(context.Context) error
		explicit bool
		org      string
	}{
		{"attach", runAttach, true, "owner"},
		{"detach", runDetach, true, "owner"},
		{"foreign cluster", runDetach, true, "other"},
		{"list in app org", runAttach, false, "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := appconfig.WithName(setupTestContext(), "app")
			flags := newAttach().Flags()
			if tc.explicit {
				require.NoError(t, flags.Parse([]string{"cluster"}))
			}
			ctx = flagctx.NewContext(ctx, flags)
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
					return &uiex.Organization{RawSlug: "owner", InternalNumericID: 7}, nil
				},
			})

			checkOwner := authtest.Organization(t, 7)
			stop := errors.New("observed scoped request")
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				GetClusterByIdFunc: func(reqCtx context.Context, id string) (mpgv2.GetClusterResponse, error) {
					checkOwner(reqCtx)
					return mpgv2.GetClusterResponse{}, stop
				},
			})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) {
					return &flaps.App{Organization: flaps.AppOrganizationInfo{Slug: "owner", InternalNumericID: 7}}, nil
				},
				GetManagedPostgresClusterFunc: func(reqCtx context.Context, id string) (flaps.ManagedPostgresCluster, error) {
					// An explicit ID may belong to another org. Preserve the original lookup context.
					require.Same(t, ctx, reqCtx)
					return flaps.ManagedPostgresCluster{ID: id, Organization: flaps.ManagedPostgresOrganization{Slug: tc.org}}, nil
				},
				ListManagedPostgresClustersFunc: func(reqCtx context.Context, req flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
					require.Equal(t, "owner", req.OrgSlug)
					checkOwner(reqCtx)
					return nil, stop
				},
				DeleteManagedPostgresAttachmentFunc: func(reqCtx context.Context, id, app string) error {
					checkOwner(reqCtx)
					return stop
				},
			})

			err := tc.run(ctx)
			if tc.org == "other" {
				require.ErrorContains(t, err, "They must be in the same organization")
			} else {
				require.ErrorIs(t, err, stop)
			}
		})
	}
}
