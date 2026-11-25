package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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
