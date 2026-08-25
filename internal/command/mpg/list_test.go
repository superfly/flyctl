package mpg

import (
	"context"
	"encoding/json"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

type organizationGraphQLClient struct{}

func (organizationGraphQLClient) MakeRequest(_ context.Context, _ *genq.Request, response *genq.Response) error {
	data := response.Data.(*gql.GetOrganizationResponse)
	data.Organization.OrganizationData = gql.OrganizationData{
		Id:       "org-id",
		Slug:     "personal",
		RawSlug:  "raw-personal",
		PaidPlan: true,
		Name:     "Personal",
		Billable: true,
	}

	return nil
}

func TestRunListUsesMergedMachinesAndLegacyResults(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("org", "personal", "")
	flags.Bool("deleted", false, "")
	ctx = flagctx.NewContext(ctx, flags)

	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GetOrganizationBySlugFunc: func(_ context.Context, slug string) (*fly.Organization, error) {
			require.Equal(t, "personal", slug)

			return &fly.Organization{Slug: slug, RawSlug: "raw-personal"}, nil
		},
		GenqClientFunc: func() genq.Client {
			return organizationGraphQLClient{}
		},
	})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresClustersFunc: func(_ context.Context, req flaps.ListManagedPostgresClustersRequest) ([]flaps.ManagedPostgresClusterSummary, error) {
			require.Equal(t, "raw-personal", req.OrgSlug)
			require.False(t, req.IncludeDeleted)

			return []flaps.ManagedPostgresClusterSummary{{
				ID: "mpg-v2", Name: "public", Region: "iad", Status: "ready", Plan: "basic",
			}}, nil
		},
	})
	ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
		ListManagedClustersFunc: func(_ context.Context, orgSlug string, deleted bool) (mpgv1.ListManagedClustersResponse, error) {
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
