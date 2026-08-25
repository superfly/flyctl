package cmdv2

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunDestroyUsesMachinesAPI(t *testing.T) {
	for _, tc := range []struct {
		name      string
		deleteErr error
		wantErr   string
	}{
		{name: "success"},
		{name: "delete failure", deleteErr: errors.New("delete failed"), wantErr: "failed to destroy cluster mpg-123: delete failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			io, _, stdout, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), io)

			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.Bool("yes", true, "")
			require.NoError(t, flags.Set("yes", "true"))
			ctx = flagctx.NewContext(ctx, flags)

			var deletedID string
			client := &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{
						ID:   "mpg-123",
						Name: "example",
						Organization: flaps.ManagedPostgresOrganization{
							Name: "Example Org",
							Slug: "example-org",
						},
					}, nil
				},
				DeleteManagedPostgresClusterFunc: func(_ context.Context, id string) error {
					deletedID = id
					return tc.deleteErr
				},
			}
			ctx = flapsutil.NewContextWithClient(ctx, client)

			err := RunDestroy(ctx, "mpg-123")
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, "mpg-123", deletedID)
			require.Contains(t, stdout.String(), "Managed Postgres cluster example (mpg-123) scheduled to be destroyed")
		})
	}
}

func TestRunDestroyDoesNotDeleteWhenLookupFails(t *testing.T) {
	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("yes", true, "")
	require.NoError(t, flags.Set("yes", "true"))
	ctx = flagctx.NewContext(ctx, flags)

	client := &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, errors.New("lookup failed")
		},
		DeleteManagedPostgresClusterFunc: func(context.Context, string) error {
			t.Fatal("DeleteManagedPostgresCluster called after failed lookup")
			return nil
		},
	}
	ctx = flapsutil.NewContextWithClient(ctx, client)

	require.EqualError(t, RunDestroy(ctx, "mpg-123"), "failed retrieving cluster mpg-123: lookup failed")
}

func TestRunDestroyFallsBackToLegacyV2APIOnPublicNotFound(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("yes", true, "")
	require.NoError(t, flags.Set("yes", "true"))
	ctx = flagctx.NewContext(ctx, flags)

	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
		},
		DeleteManagedPostgresClusterFunc: func(context.Context, string) error {
			t.Fatal("public delete called after public lookup returned not found")
			return nil
		},
	})

	var destroyedOrg, destroyedID string
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
			require.Equal(t, "mpg-123", id)
			return mpgv2.GetClusterResponse{Data: mpgv2.ManagedCluster{
				Id:   id,
				Name: "example",
				Organization: fly.Organization{
					Name: "Example Org",
					Slug: "example-org",
				},
			}}, nil
		},
		DestroyClusterFunc: func(_ context.Context, orgSlug, id string) error {
			destroyedOrg, destroyedID = orgSlug, id
			return nil
		},
	})

	require.NoError(t, RunDestroy(ctx, "mpg-123"))
	require.Equal(t, "example-org", destroyedOrg)
	require.Equal(t, "mpg-123", destroyedID)
	require.Contains(t, stdout.String(), "Managed Postgres cluster example (mpg-123) scheduled to be destroyed")
}

func TestRunDestroyRequiresYesWhenNonInteractive(t *testing.T) {
	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("yes", false, "")
	ctx = flagctx.NewContext(ctx, flags)

	client := &mock.FlapsClient{
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{
				ID:   "mpg-123",
				Name: "example",
				Organization: flaps.ManagedPostgresOrganization{
					Name: "Example Org",
				},
			}, nil
		},
		DeleteManagedPostgresClusterFunc: func(context.Context, string) error {
			t.Fatal("DeleteManagedPostgresCluster called without confirmation")
			return nil
		},
	}
	ctx = flapsutil.NewContextWithClient(ctx, client)

	require.ErrorContains(t, RunDestroy(ctx, "mpg-123"), "--yes flag must be specified")
}
