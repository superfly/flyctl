package cmdv2

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func detachTestContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()

	return iostreams.NewContext(context.Background(), io), stdout
}

func TestRunDetach(t *testing.T) {
	const wantSuccessOutput = "\nPostgres cluster mpg-123 has been detached from my-app\n" +
		"Note: This only removes the attachment record. Any secrets (like DATABASE_URL) are still set on the app.\n" +
		"Use 'fly secrets unset DATABASE_URL -a my-app' to remove the connection string.\n"

	tests := []struct {
		name             string
		publicDeleteErr  error
		wantPublicDelete bool
		wantErr          string
	}{
		{
			name:             "uses Machines API",
			wantPublicDelete: true,
		},
		{
			name:             "returns Machines API delete failure",
			publicDeleteErr:  errors.New("delete failed"),
			wantPublicDelete: true,
			wantErr:          "failed to detach: delete failed",
		},
		{
			name:             "404 propagated",
			publicDeleteErr:  &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("attachment not found")},
			wantPublicDelete: true,
			wantErr:          "failed to detach: attachment not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := detachTestContext(t)
			publicDeleteCalled := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				DeleteManagedPostgresAttachmentFunc: func(_ context.Context, id, appName string) error {
					publicDeleteCalled = true
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "my-app", appName)

					return test.publicDeleteErr
				},
			})

			err := RunDetach(ctx, "mpg-123", "my-app")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Empty(t, stdout.String(), "no success output on error")
			} else {
				require.NoError(t, err)
				require.Equal(t, wantSuccessOutput, stdout.String(), "success output must be byte-identical")
			}
			require.Equal(t, test.wantPublicDelete, publicDeleteCalled)
		})
	}
}
