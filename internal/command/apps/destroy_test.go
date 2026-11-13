package apps

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunDestroySafeguards(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing apps", nil, "no app names provided"},
		{"headless requires confirmation", []string{"production"}, "yes flag must be specified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newDestroy()
			require.NoError(t, cmd.ParseFlags(tc.args))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			calls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				DeleteAppFunc: func(context.Context, string) error { calls++; return nil },
			})
			require.ErrorContains(t, RunDestroy(ctx), tc.want)
			require.Zero(t, calls)
			require.Empty(t, out.String())
		})
	}
}

func TestRunDestroyBatch(t *testing.T) {
	denied := errors.New("delete denied")
	for _, tc := range []struct {
		name, failAt string
	}{
		{"success", ""}, {"failure stops the batch", "two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("FLY_CONFIG_DIR", dir)
			old := viper.Get(flyctl.ConfigAppSecretsMinvers)
			t.Cleanup(func() { viper.Set(flyctl.ConfigAppSecretsMinvers, old) })
			viper.Set(flyctl.ConfigAppSecretsMinvers, map[string]uint64{"one": 1, "two": 2, "three": 3, "unrelated": 4})
			cmd := newDestroy()
			require.NoError(t, cmd.ParseFlags([]string{"--yes", "one", "two", "three"}))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			ctx = state.WithConfigDirectory(ctx, dir)
			var deleted []string
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
					return &flaps.App{Name: name, Organization: flaps.AppOrganizationInfo{Slug: "owner"}}, nil
				},
				DeleteAppFunc: func(_ context.Context, name string) error {
					deleted = append(deleted, name)
					if name == tc.failAt {
						return denied
					}
					return nil
				},
			})
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
					require.Equal(t, "owner", slug)
					return &uiex.Organization{Slug: slug}, nil
				},
			})
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
				GenqClientFunc: func() graphql.Client {
					return mock.GraphQLClient(func(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
						require.Equal(t, "ListOrganizationAddOns", req.OpName)
						return json.Unmarshal([]byte(`{"organization":{"addOns":{"nodes":[]}}}`), resp.Data)
					})
				},
			})
			err := RunDestroy(ctx)
			if tc.failAt == "" {
				require.NoError(t, err)
				require.Equal(t, []string{"one", "two", "three"}, deleted)
				require.Equal(t, "Destroyed app one\nDestroyed app two\nDestroyed app three\n", out.String())
			} else {
				require.ErrorIs(t, err, denied)
				require.Equal(t, []string{"one", "two"}, deleted)
				require.Equal(t, "Destroyed app one\n", out.String())
				version, err := appsecrets.GetMinvers("two")
				require.NoError(t, err)
				require.NotNil(t, version)
				require.Equal(t, uint64(2), *version)
			}
			version, err := appsecrets.GetMinvers("one")
			require.NoError(t, err)
			require.Nil(t, version)
			version, err = appsecrets.GetMinvers("unrelated")
			require.NoError(t, err)
			require.NotNil(t, version)
			require.Equal(t, uint64(4), *version)
		})
	}
}
