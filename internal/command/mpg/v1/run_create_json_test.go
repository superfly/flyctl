package cmdv1

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

// The JSON failure message must name what the server created, which may differ
// from the requested name, and fall back to the request when it is empty.
func TestCreateJSONFailureNamesCreatedResource(t *testing.T) {
	for _, tc := range []struct {
		kind, flag, returned, want string
		run                        func(context.Context, string) error
	}{
		{"user", "username", "normalized", "user normalized in cluster c1", RunUsersCreate},
		{"user", "username", "", "user requested in cluster c1", RunUsersCreate},
		{"database", "name", "normalized", "database normalized in cluster c1", RunDatabasesCreate},
		{"database", "name", "", "database requested in cluster c1", RunDatabasesCreate},
	} {
		t.Run(tc.kind+"/"+tc.returned, func(t *testing.T) {
			streams, _, _, _ := iostreams.Test()
			streams.Out = failingWriter{}
			ctx := iostreams.NewContext(context.Background(), streams)
			ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
			fs := pflag.NewFlagSet("create", pflag.ContinueOnError)
			fs.String("username", "", "")
			fs.String("role", "", "")
			fs.String("name", "", "")
			require.NoError(t, fs.Set(tc.flag, "requested"))
			require.NoError(t, fs.Set("role", "reader"))
			ctx = flagctx.NewContext(ctx, fs)
			ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
				CreateUserWithRoleFunc: func(context.Context, string, mpgv1.CreateUserWithRoleInput) (mpgv1.CreateUserWithRoleResponse, error) {
					var r mpgv1.CreateUserWithRoleResponse
					r.Data.Name = tc.returned
					return r, nil
				},
				CreateDatabaseFunc: func(context.Context, string, mpgv1.CreateDatabaseInput) (mpgv1.CreateDatabaseResponse, error) {
					var r mpgv1.CreateDatabaseResponse
					r.Data.Name = tc.returned
					return r, nil
				},
			})
			err := tc.run(ctx, "c1")
			require.ErrorContains(t, err, tc.want+" was created")
		})
	}
}
