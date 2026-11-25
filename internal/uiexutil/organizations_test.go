package uiexutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/requestauth"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

func TestAppOrganizationUsesAppOwner(t *testing.T) {
	checkOwner := authtest.Organization(t, 7)
	ctx := requestauth.WithOrganizations(context.Background(), 8)
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(ctx context.Context, slug string) (*uiex.Organization, error) {
			require.Equal(t, "owner", slug)
			checkOwner(ctx)
			return &uiex.Organization{Slug: slug}, nil
		},
	})
	app := &flaps.App{Organization: flaps.AppOrganizationInfo{Slug: "owner", InternalNumericID: 7}}
	org, err := uiexutil.AppOrganization(ctx, app)
	require.NoError(t, err)
	require.Equal(t, "owner", org.Slug)
}
