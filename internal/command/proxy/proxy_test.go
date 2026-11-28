package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// Without an app there is no default remote host.
func TestProxyWithoutAppRequiresRemoteHost(t *testing.T) {
	t.Setenv("FLY_ORG", "")

	cmd := New()
	require.NoError(t, cmd.Flags().Parse([]string{"5432"}))

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(t.Context(), io)
	ctx = config.NewContext(ctx, &config.Config{})
	ctx = appconfig.WithName(ctx, "")
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{{Slug: "acme"}, {Slug: "other"}}, nil
		},
	})

	require.ErrorContains(t, run(flag.NewContext(ctx, cmd.Flags())), "remote host argument is required")
}

// The paths `fly proxy` is normally run on must get past the remote-host check:
// with an app, the default remote host is <app>.internal; without one, the
// remote host is given. Each stops at the first API call after the check.
func TestProxyRunsWithAppOrRemoteHost(t *testing.T) {
	errGetApp := errors.New("stop: looked up the app")
	for _, tc := range []struct {
		name, app string
		args      []string
		want      string
	}{
		{"app, default remote host", "my-app", []string{"5432"}, errGetApp.Error()},
		{"app and remote host", "my-app", []string{"5432", "db.internal"}, errGetApp.Error()},
		{"remote host without an app", "", []string{"5432", "db.internal"}, "org slug must be specified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLY_ORG", "")

			cmd := New()
			require.NoError(t, cmd.Flags().Parse(tc.args))

			io, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), io)
			ctx = config.NewContext(ctx, &config.Config{})
			ctx = appconfig.WithName(ctx, tc.app)
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
					return []uiex.Organization{{Slug: "acme"}, {Slug: "other"}}, nil
				},
			})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) { return nil, errGetApp },
			})

			err := run(flag.NewContext(ctx, cmd.Flags()))
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "remote host argument is required")
		})
	}
}
