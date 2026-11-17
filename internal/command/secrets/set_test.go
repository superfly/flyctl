package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func TestSecretUpdateAndAppRefreshUseResolvedOrg(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	previous := viper.Get(flyctl.ConfigAppSecretsMinvers)
	t.Cleanup(func() { viper.Set(flyctl.ConfigAppSecretsMinvers, previous) })
	check := authtest.Organization(t, 7)
	stop := errors.New("stop after app refresh")
	app := &flaps.App{Name: "test-app", Organization: flaps.AppOrganizationInfo{InternalNumericID: 7}}
	client := &mock.FlapsClient{
		UpdateAppSecretsFunc: func(ctx context.Context, name string, _ map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			check(ctx)
			require.Equal(t, app.Name, name)
			return &fly.UpdateAppSecretsResp{}, nil
		},
		GetAppFunc: func(ctx context.Context, name string) (*flaps.App, error) {
			check(ctx)
			require.Equal(t, app.Name, name)
			return nil, stop
		},
	}
	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(t.Context(), ios)
	ctx = state.WithConfigDirectory(ctx, dir)
	ctx = flapsutil.NewContextWithClient(ctx, client)
	require.ErrorIs(t, SetSecretsAndDeploy(ctx, client, app, map[string]string{"KEY": "test"}, DeploymentArgs{}), stop)
}
