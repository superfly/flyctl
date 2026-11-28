package mpg

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/requestauth/authtest"
	"github.com/superfly/flyctl/internal/uiex"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunListUsesMergedMachinesAndLegacyResults(t *testing.T) {
	assertOrg := authtest.Organization(t, 7)
	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("org", "personal", "")
	flags.Bool("deleted", false, "")
	ctx = flagctx.NewContext(ctx, flags)

	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
			require.Equal(t, "personal", slug)

			return &uiex.Organization{Slug: slug, RawSlug: "raw-personal", InternalNumericID: 7}, nil
		},
	})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(ctx context.Context, req flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			assertOrg(ctx)
			require.Equal(t, "raw-personal", req.OrgSlug)
			require.False(t, req.IncludeDeleted)

			return []flaps.ManagedPostgresClusterSummary{{
				ID: "mpg-v2", Name: "public", Region: "iad", Status: "ready", Plan: "basic",
			}}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		ListManagedClustersFunc: func(ctx context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
			assertOrg(ctx)
			require.Equal(t, "raw-personal", orgSlug)
			require.False(t, deleted)

			return mpgv1.ListManagedClustersResponse{Data: []mpgv1.ManagedCluster{
				{Id: "mpg-v2", ClusterId: "fly-mpg-v2", Name: "legacy duplicate", Version: 2},
				{Id: "mpg-v1", Name: "legacy", Region: "ord", Status: "ready", Plan: "basic", Version: 1},
			}}, nil
		},
	})

	require.NoError(t, runList(ctx))
	var clusters []mpgv1.ManagedCluster
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &clusters))
	require.Len(t, clusters, 2)
	require.Equal(t, "mpg-v2", clusters[0].Id)
	require.Equal(t, "fly-mpg-v2", clusters[0].ClusterId)
	require.Equal(t, 2, clusters[0].Version)
	require.Equal(t, "public", clusters[0].Name)
	require.Equal(t, "raw-personal", clusters[0].Organization.Slug)
	require.Equal(t, "mpg-v1", clusters[1].Id)
	require.Equal(t, 1, clusters[1].Version)
}

func TestListJSONOrganizationName(t *testing.T) {
	for _, legacyName := range []string{"", "My Org"} {
		t.Run("legacy-name="+legacyName, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ctx := config.NewContext(iostreams.NewContext(context.Background(), ios), &config.Config{JSONOutput: true})
			cmd := newList()
			require.NoError(t, cmd.Flags().Parse([]string{"--org", "my-org", "--json"}))
			ctx = flagctx.NewContext(ctx, cmd.Flags())
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
					return &uiex.Organization{Name: "My Org", Slug: "my-org", RawSlug: "my-org"}, nil
				},
			})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListManagedPostgresClustersFunc: func(context.Context, flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
					return []flaps.ManagedPostgresClusterSummary{{ID: "new-id"}}, nil
				},
			})
			ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
				ListManagedClustersFunc: func(context.Context, string, bool) (mpgv1.ListManagedClustersResponse, error) {
					var clusters []mpgv1.ManagedCluster
					if legacyName != "" {
						clusters = append(clusters, mpgv1.ManagedCluster{Id: "new-id", Organization: fly.Organization{Name: legacyName, Slug: "my-org"}})
					}
					return mpgv1.ListManagedClustersResponse{Data: clusters}, nil
				},
			})

			require.NoError(t, runList(ctx))
			var clusters []mpgv1.ManagedCluster
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &clusters))
			require.Len(t, clusters, 1)
			require.Equal(t, "new-id", clusters[0].Id)
			require.Equal(t, "my-org", clusters[0].Organization.Slug)
			require.Equal(t, legacyName, clusters[0].Organization.Name)
		})
	}
}
