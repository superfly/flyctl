package consul

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestAttachReportsEnableFailure(t *testing.T) {
	errEnable := errors.New("not authorized")

	ctx := flag.NewContext(t.Context(), newAttach().Flags())
	ctx = appconfig.WithName(ctx, "test-app")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
			return &flaps.App{Name: name}, nil
		},
	})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		EnablePostgresConsulFunc: func(context.Context, string) (*fly.PostgresEnableConsulPayload, error) {
			return nil, errEnable
		},
	})

	require.ErrorIs(t, runAttach(ctx), errEnable)
}
