package cmdv2

import (
	"bytes"
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

func destroyTestContext(t *testing.T, yes bool) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("yes", yes, "")

	return flagctx.NewContext(ctx, flags), stdout
}

func TestRunDestroy(t *testing.T) {
	publicCluster := flaps.ManagedPostgresCluster{
		ID:   "mpg-123",
		Name: "example",
		Organization: flaps.ManagedPostgresOrganization{
			Name: "Example Org",
			Slug: "example-org",
		},
	}
	legacyCluster := mpgv2.ManagedCluster{
		Id:   "mpg-123",
		Name: "example",
		Organization: fly.Organization{
			Name: "Example Org",
			Slug: "example-org",
		},
	}

	tests := []struct {
		name             string
		yes              bool
		publicCluster    flaps.ManagedPostgresCluster
		publicLookupErr  error
		publicDeleteErr  error
		legacyCluster    mpgv2.ManagedCluster
		wantPublicDelete bool
		wantLegacy       bool
		wantErr          string
		wantOutput       string
	}{
		{
			name:             "uses Machines API",
			yes:              true,
			publicCluster:    publicCluster,
			wantPublicDelete: true,
			wantOutput:       "Managed Postgres cluster example (mpg-123) scheduled to be destroyed",
		},
		{
			name:             "returns Machines API delete failure",
			yes:              true,
			publicCluster:    publicCluster,
			publicDeleteErr:  errors.New("delete failed"),
			wantPublicDelete: true,
			wantErr:          "failed to destroy cluster mpg-123: delete failed",
		},
		{
			name:            "does not delete when lookup fails",
			yes:             true,
			publicLookupErr: errors.New("lookup failed"),
			wantErr:         "failed retrieving cluster mpg-123: lookup failed",
		},
		{
			name:            "falls back to legacy API on public not found",
			yes:             true,
			publicLookupErr: flaps.ErrFlapsNotFound,
			legacyCluster:   legacyCluster,
			wantLegacy:      true,
			wantOutput:      "Managed Postgres cluster example (mpg-123) scheduled to be destroyed",
		},
		{
			name:          "requires yes when non-interactive",
			publicCluster: publicCluster,
			wantErr:       "--yes flag must be specified",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := destroyTestContext(t, test.yes)
			publicDeleteCalled := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
					require.Equal(t, "mpg-123", id)

					return test.publicCluster, test.publicLookupErr
				},
				DeleteManagedPostgresClusterFunc: func(_ context.Context, id string) error {
					publicDeleteCalled = true
					require.Equal(t, "mpg-123", id)

					return test.publicDeleteErr
				},
			})

			legacyCalled := false
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
					legacyCalled = true
					require.Equal(t, "mpg-123", id)

					return mpgv2.GetClusterResponse{Data: test.legacyCluster}, nil
				},
				DestroyClusterFunc: func(_ context.Context, orgSlug, id string) error {
					legacyCalled = true
					require.Equal(t, "example-org", orgSlug)
					require.Equal(t, "mpg-123", id)

					return nil
				},
			})

			err := RunDestroy(ctx, "mpg-123")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantPublicDelete, publicDeleteCalled)
			require.Equal(t, test.wantLegacy, legacyCalled)
			if test.wantOutput != "" {
				require.Contains(t, stdout.String(), test.wantOutput)
			}
		})
	}
}
