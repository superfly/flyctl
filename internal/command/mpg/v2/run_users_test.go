package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// usersTestContext builds a context that mirrors the shape used by the mpg/v2
// command package: a flag set for the user/role/yes flags, an iostreams pair
// for table/JSON output, and the JSON output toggle carried on the config.
func usersTestContext(t *testing.T, jsonOutput bool) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, stderr := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("username", "", "")
	flags.String("role", "", "")
	flags.Bool("yes", false, "")

	return flagctx.NewContext(ctx, flags), stdout, stderr
}

func TestRunUsersList(t *testing.T) {
	tests := []struct {
		name         string
		jsonOutput   bool
		publicUsers  []flaps.ManagedPostgresUser
		publicErr    error
		wantErr      string
		wantContains []string
		wantJSON     string
	}{
		{name: "public table", publicUsers: []flaps.ManagedPostgresUser{{Username: "app_user", Role: flaps.ManagedPostgresUserRoleWriter}}, wantContains: []string{"NAME", "ROLE", "app_user", "writer"}},
		{name: "public JSON keeps legacy name shape", jsonOutput: true, publicUsers: []flaps.ManagedPostgresUser{{Username: "app_user", Role: flaps.ManagedPostgresUserRoleReader}}, wantJSON: `[{"name":"app_user","role":"reader"}]`},
		{name: "public empty", wantContains: []string{"No users found for cluster mpg-123"}},
		{name: "non-404 authoritative", publicErr: &flaps.FlapsError{ResponseStatusCode: 422, OriginalError: errors.New("invalid")}, wantErr: "failed to list users for cluster mpg-123"},
		{name: "404 propagated", publicErr: &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("cluster not found")}, wantErr: "failed to list users for cluster mpg-123: cluster not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, tt.jsonOutput)
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{ListManagedPostgresUsersFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresUser, error) {
				publicCalls++
				require.Equal(t, "mpg-123", id)

				return tt.publicUsers, tt.publicErr
			}})

			err := RunUsersList(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, publicCalls)
			for _, want := range tt.wantContains {
				require.Contains(t, stdout.String(), want)
			}
			if tt.wantJSON != "" {
				var got, want any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
				require.NoError(t, json.Unmarshal([]byte(tt.wantJSON), &want))
				require.Equal(t, want, got)
				require.NotContains(t, stdout.String(), "username")
			}
			require.NotContains(t, strings.ToLower(stdout.String()), "password")
			require.NotContains(t, strings.ToLower(stdout.String()), "credential")
		})
	}
}

func TestRunUsersCreateRoutingAndValidation(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		role       string
		publicErr  error
		wantPublic bool
		wantErr    string
	}{
		{name: "public success", username: "new_user", role: "writer", wantPublic: true},
		{name: "conflict authoritative", username: "new_user", role: "writer", publicErr: &flaps.FlapsError{ResponseStatusCode: 409, OriginalError: errors.New("exists")}, wantPublic: true, wantErr: "failed to create user"},
		{name: "username required first", role: "writer", wantErr: "username must be specified"},
		{name: "role required", username: "new_user", wantErr: "user role must be specified"},
		{name: "role validated before API", username: "new_user", role: "owner", wantErr: `invalid role "owner"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, false)
			if tt.username != "" {
				require.NoError(t, flag.SetString(ctx, "username", tt.username))
			}
			if tt.role != "" {
				require.NoError(t, flag.SetString(ctx, "role", tt.role))
			}
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{CreateManagedPostgresUserFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresUserRequest) (flaps.ManagedPostgresUser, error) {
				publicCalls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, flaps.CreateManagedPostgresUserRequest{Username: tt.username, Role: tt.role}, req)

				return flaps.ManagedPostgresUser{Username: tt.username, Role: flaps.ManagedPostgresUserRole(tt.role)}, tt.publicErr
			}})
			err := RunUsersCreate(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "User created successfully!")
			}
			require.Equal(t, tt.wantPublic, publicCalls == 1)
		})
	}
}

func TestRunUsersSetRoleRouting(t *testing.T) {
	for _, tt := range []struct {
		name         string
		publicErr    error
		wantErr      string
		wantExactErr string
	}{
		{name: "public success"},
		{name: "server error authoritative", publicErr: &flaps.FlapsError{ResponseStatusCode: 500, OriginalError: errors.New("boom")}, wantErr: "failed to update user role"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, false)
			require.NoError(t, flag.SetString(ctx, "username", "app_user"))
			require.NoError(t, flag.SetString(ctx, "role", "reader"))
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{UpdateManagedPostgresUserRoleFunc: func(_ context.Context, id, username string, req flaps.UpdateManagedPostgresUserRoleRequest) error {
				publicCalls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, "app_user", username)
				require.Equal(t, "reader", req.Role)

				return tt.publicErr
			}})
			err := RunUsersSetRole(ctx, "mpg-123")
			if tt.wantExactErr != "" {
				require.EqualError(t, err, tt.wantExactErr)
				require.NotContains(t, stdout.String(), "User role updated successfully!")
			} else if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "User role updated successfully!")
			}
			require.Equal(t, 1, publicCalls)
		})
	}
}

func TestRunUsersDeleteRoutingAndYes(t *testing.T) {
	for _, tt := range []struct {
		name         string
		publicErr    error
		wantErr      string
		wantExactErr string
	}{
		{name: "public success"},
		{name: "gone authoritative", publicErr: &flaps.FlapsError{ResponseStatusCode: 410, OriginalError: errors.New("gone")}, wantErr: "failed to delete user"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, stderr := usersTestContext(t, false)
			require.NoError(t, flag.SetString(ctx, "username", "old_user"))
			require.NoError(t, flag.FromContext(ctx).Set("yes", "true"))
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{DeleteManagedPostgresUserFunc: func(_ context.Context, id, username string) error {
				publicCalls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, "old_user", username)

				return tt.publicErr
			}})
			err := RunUsersDelete(ctx, "mpg-123")
			if tt.wantExactErr != "" {
				require.EqualError(t, err, tt.wantExactErr)
				require.NotContains(t, stdout.String(), "deleted successfully")
			} else if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "User old_user deleted successfully from cluster mpg-123")
			}
			require.Empty(t, stderr.String())
			require.Equal(t, 1, publicCalls)
		})
	}
}
