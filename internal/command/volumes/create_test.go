package volumes

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestCreateVolumeUsesAppOwner(t *testing.T) {
	checkOwner := authtest.Organization(t, 7)
	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = appconfig.WithName(config.NewContext(ctx, &config.Config{}), "app")
	flags := newCreate().Flags()
	require.NoError(t, flags.Parse([]string{"--yes", "--region", "iad", "--snapshot-id", "snap", "data"}))
	ctx = flagctx.NewContext(ctx, flags)
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
			return &uiex.Organization{InternalNumericID: 7}, nil
		},
	})
	stop := errors.New("stop at create")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(context.Context, string) (*flaps.App, error) {
			return &flaps.App{Name: "app", Organization: flaps.AppOrganizationInfo{Slug: "org", InternalNumericID: 7}}, nil
		},
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad"}}}, nil
		},
		CreateVolumeFunc: func(ctx context.Context, app string, req fly.CreateVolumeRequest) (*fly.Volume, error) {
			require.Equal(t, "app", app)
			require.Equal(t, "snap", *req.SnapshotID)
			checkOwner(ctx)
			return nil, stop
		},
	})
	require.ErrorIs(t, runCreate(ctx), stop)
}
