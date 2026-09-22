package cmdv2

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
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
	tests := []struct {
		name             string
		yes              bool
		publicCluster    flaps.ManagedPostgresCluster
		publicLookupErr  error
		publicDeleteErr  error
		wantPublicDelete bool
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
			name:             "404 propagated on delete",
			yes:              true,
			publicCluster:    publicCluster,
			publicDeleteErr:  &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("cluster not found")},
			wantPublicDelete: true,
			wantErr:          "failed to destroy cluster mpg-123: cluster not found",
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

			err := RunDestroy(ctx, "mpg-123")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantPublicDelete, publicDeleteCalled)
			if test.wantOutput != "" {
				require.Contains(t, stdout.String(), test.wantOutput)
			}
		})
	}
}
