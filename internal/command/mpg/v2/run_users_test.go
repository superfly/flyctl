package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
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
	flags.Bool("kill-sessions", false, "")

	return flagctx.NewContext(ctx, flags), stdout, stderr
}

func wrappedNotFound() error {
	return fmt.Errorf("public users: %w", &flaps.FlapsError{
		ResponseStatusCode: 404,
		OriginalError:      errors.New("not found"),
	})
}

func TestRunUsersList(t *testing.T) {
	tests := []struct {
		name         string
		jsonOutput   bool
		publicUsers  []flaps.ManagedPostgresUser
		publicErr    error
		legacyUsers  []mpgv2.User
		legacyErr    error
		wantLegacy   bool
		wantErr      string
		wantContains []string
		wantJSON     string
	}{
		{name: "public table", publicUsers: []flaps.ManagedPostgresUser{{Username: "app_user", Role: flaps.ManagedPostgresUserRoleWriter}}, wantContains: []string{"NAME", "ROLE", "app_user", "writer"}},
		{name: "public JSON keeps legacy name shape", jsonOutput: true, publicUsers: []flaps.ManagedPostgresUser{{Username: "app_user", Role: flaps.ManagedPostgresUserRoleReader}}, wantJSON: `[{"name":"app_user","role":"reader"}]`},
		{name: "public empty", wantContains: []string{"No users found for cluster mpg-123"}},
		{name: "wrapped 404 fallback", publicErr: wrappedNotFound(), legacyUsers: []mpgv2.User{{Name: "legacy_user", Role: "schema_admin"}}, wantLegacy: true, wantContains: []string{"legacy_user", "schema_admin"}},
		{name: "non-404 authoritative", publicErr: &flaps.FlapsError{ResponseStatusCode: 422, OriginalError: errors.New("invalid")}, wantErr: "failed to list users for cluster mpg-123", wantLegacy: false},
		{name: "legacy failure", publicErr: flaps.ErrFlapsNotFound, legacyErr: errors.New("legacy boom"), wantLegacy: true, wantErr: "failed to list users for cluster mpg-123: legacy boom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, tt.jsonOutput)
			publicCalls, legacyCalls := 0, 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{ListManagedPostgresUsersFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresUser, error) {
				publicCalls++
				require.Equal(t, "mpg-123", id)

				return tt.publicUsers, tt.publicErr
			}})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{ListUsersFunc: func(_ context.Context, id string) (mpgv2.ListUsersResponse, error) {
				legacyCalls++
				require.Equal(t, "mpg-123", id)

				return mpgv2.ListUsersResponse{Data: tt.legacyUsers}, tt.legacyErr
			}})

			err := RunUsersList(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
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
		legacyErr  error
		wantPublic bool
		wantLegacy bool
		wantErr    string
	}{
		{name: "public success", username: "new_user", role: "writer", wantPublic: true},
		{name: "wrapped 404 fallback", username: "new_user", role: "reader", publicErr: wrappedNotFound(), wantPublic: true, wantLegacy: true},
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
			publicCalls, legacyCalls := 0, 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{CreateManagedPostgresUserFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresUserRequest) (flaps.ManagedPostgresUser, error) {
				publicCalls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, flaps.CreateManagedPostgresUserRequest{Username: tt.username, Role: tt.role}, req)

				return flaps.ManagedPostgresUser{Username: tt.username, Role: flaps.ManagedPostgresUserRole(tt.role)}, tt.publicErr
			}})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{CreateUserWithRoleFunc: func(_ context.Context, id string, input mpgv2.CreateUserWithRoleInput) (mpgv2.CreateUserWithRoleResponse, error) {
				legacyCalls++
				require.Equal(t, mpgv2.CreateUserWithRoleInput{Username: tt.username, Role: tt.role}, input)

				return mpgv2.CreateUserWithRoleResponse{Data: mpgv2.User{Name: tt.username, Role: tt.role}}, tt.legacyErr
			}})
			err := RunUsersCreate(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "User created successfully!")
			}
			require.Equal(t, tt.wantPublic, publicCalls == 1)
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
		})
	}
}

func TestRunUsersSetRoleRouting(t *testing.T) {
	for _, tt := range []struct {
		name         string
		publicErr    error
		legacyErr    error
		wantLegacy   bool
		wantErr      string
		wantExactErr string
	}{
		{name: "public success"},
		{name: "wrapped 404 fallback", publicErr: wrappedNotFound(), wantLegacy: true},
		{name: "wrapped 404 legacy failure", publicErr: wrappedNotFound(), legacyErr: errors.New("legacy update boom"), wantLegacy: true, wantExactErr: "failed to update user role: legacy update boom"},
		{name: "server error authoritative", publicErr: &flaps.FlapsError{ResponseStatusCode: 500, OriginalError: errors.New("boom")}, wantErr: "failed to update user role"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, false)
			require.NoError(t, flag.SetString(ctx, "username", "app_user"))
			require.NoError(t, flag.SetString(ctx, "role", "reader"))
			publicCalls, legacyCalls := 0, 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{UpdateManagedPostgresUserRoleFunc: func(_ context.Context, id, username string, req flaps.UpdateManagedPostgresUserRoleRequest) error {
				publicCalls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, "app_user", username)
				require.Equal(t, "reader", req.Role)

				return tt.publicErr
			}})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{UpdateUserRoleFunc: func(_ context.Context, id, username string, input mpgv2.UpdateUserRoleInput) error {
				legacyCalls++
				require.Equal(t, "reader", input.Role)

				return tt.legacyErr
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
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
		})
	}
}

func TestRunUsersDeleteRoutingAndYes(t *testing.T) {
	for _, tt := range []struct {
		name         string
		publicErr    error
		legacyErr    error
		wantLegacy   bool
		wantErr      string
		wantExactErr string
	}{
		{name: "public success"},
		{name: "classified 404 fallback", publicErr: flaps.ErrFlapsNotFound, wantLegacy: true},
		{name: "wrapped 404 legacy failure", publicErr: wrappedNotFound(), legacyErr: errors.New("legacy delete boom"), wantLegacy: true, wantExactErr: "failed to delete user: legacy delete boom"},
		{name: "gone authoritative", publicErr: &flaps.FlapsError{ResponseStatusCode: 410, OriginalError: errors.New("gone")}, wantErr: "failed to delete user"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, stderr := usersTestContext(t, false)
			require.NoError(t, flag.SetString(ctx, "username", "old_user"))
			require.NoError(t, flag.FromContext(ctx).Set("yes", "true"))
			publicCalls, legacyCalls := 0, 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{DeleteManagedPostgresUserFunc: func(_ context.Context, id, username string) error {
				publicCalls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, "old_user", username)

				return tt.publicErr
			}})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{DeleteUserFunc: func(_ context.Context, id, username string) error {
				legacyCalls++

				return tt.legacyErr
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
			require.Equal(t, tt.wantLegacy, legacyCalls == 1)
		})
	}
}

func TestUsersInteractiveListUsesSameFallbackPolicy(t *testing.T) {
	ctx, _, _ := usersTestContext(t, false)
	publicCalls, legacyCalls := 0, 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{ListManagedPostgresUsersFunc: func(context.Context, string) ([]flaps.ManagedPostgresUser, error) {
		publicCalls++

		return nil, wrappedNotFound()
	}})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{ListUsersFunc: func(context.Context, string) (mpgv2.ListUsersResponse, error) {
		legacyCalls++

		return mpgv2.ListUsersResponse{Data: []mpgv2.User{{Name: "select_user", Role: "writer"}}}, nil
	}})
	users, err := listUsers(ctx, "mpg-123")
	require.NoError(t, err)
	require.Equal(t, []mpgv2.User{{Name: "select_user", Role: "writer"}}, users)
	require.Equal(t, 1, publicCalls)
	require.Equal(t, 1, legacyCalls)
}

func TestRunUsersRotatePassword(t *testing.T) {
	tests := []struct {
		name               string
		username           string
		killSessions       bool
		jsonOutput         bool
		credentials        flaps.ManagedPostgresUserCredentials
		rotateErr          error
		wantErr            string
		wantContains       []string
		wantNotContains    []string
		wantErrContains    []string
		wantErrNotContains []string
		wantJSON           string
	}{
		{
			name:            "success table output",
			username:        "app_user",
			killSessions:    false,
			credentials:     flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "newpass123"},
			wantContains:    []string{"Password rotated successfully!", "app_user", "newpass123"},
			wantNotContains: []string{"were terminated"},
		},
		{
			name:         "success with kill-sessions",
			username:     "app_user",
			killSessions: true,
			credentials:  flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "newpass456"},
			wantContains: []string{"Password rotated successfully!", "app_user", "newpass456", "Existing sessions for app_user were terminated."},
		},
		{
			name:        "JSON output",
			username:    "app_user",
			jsonOutput:  true,
			credentials: flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "newpass789"},
			wantJSON:    `{"username":"app_user","password":"newpass789"}`,
		},
		{
			name:            "error without kill-sessions",
			username:        "app_user",
			killSessions:    false,
			rotateErr:       errors.New("connection failed"),
			wantErr:         "failed to rotate password for user app_user",
			wantErrContains: []string{"may already have been rotated"},
		},
		{
			name:            "error with kill-sessions",
			username:        "app_user",
			killSessions:    true,
			rotateErr:       errors.New("connection failed"),
			wantErr:         "failed to rotate password for user app_user",
			wantErrContains: []string{"may already have been rotated", "session termination is a separate step"},
		},
		{
			name:    "non-interactive without username",
			wantErr: "username must be specified with --username flag when not running interactively",
		},
		{
			name:               "404 not found",
			username:           "app_user",
			rotateErr:          wrappedNotFound(),
			wantErr:            "failed to rotate password for user app_user",
			wantErrNotContains: []string{"may already have been rotated"},
		},
		{
			name:               "422 validation error",
			username:           "app_user",
			rotateErr:          &flaps.FlapsError{ResponseStatusCode: 422, OriginalError: errors.New("invalid input")},
			wantErr:            "failed to rotate password for user app_user",
			wantErrNotContains: []string{"may already have been rotated"},
		},
		{
			name:            "500 server error is ambiguous",
			username:        "app_user",
			rotateErr:       &flaps.FlapsError{ResponseStatusCode: 500, OriginalError: errors.New("internal server error")},
			wantErr:         "failed to rotate password for user app_user",
			wantErrContains: []string{"may already have been rotated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, stderr := usersTestContext(t, tt.jsonOutput)
			require.NoError(t, flag.FromContext(ctx).Set("yes", "true"))
			if tt.username != "" {
				require.NoError(t, flag.SetString(ctx, "username", tt.username))
			}
			if tt.killSessions {
				require.NoError(t, flag.FromContext(ctx).Set("kill-sessions", "true"))
			}

			rotateCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{RotateManagedPostgresUserPasswordFunc: func(_ context.Context, id, username string, req flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
				rotateCalls++
				require.Equal(t, "mpg-123", id)
				if tt.username != "" {
					require.Equal(t, tt.username, username)
				}
				require.Equal(t, tt.killSessions, req.KillSessions)

				return tt.credentials, tt.rotateErr
			}})

			err := RunUsersRotatePassword(ctx, "mpg-123")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if err != nil {
				for _, want := range tt.wantErrContains {
					require.Contains(t, stderr.String(), want)
				}
				for _, notWant := range tt.wantErrNotContains {
					require.NotContains(t, stderr.String(), notWant)
				}
			}

			for _, want := range tt.wantContains {
				require.Contains(t, stdout.String(), want)
			}
			for _, notWant := range tt.wantNotContains {
				require.NotContains(t, stdout.String(), notWant)
			}

			if tt.wantJSON != "" {
				var got, want any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
				require.NoError(t, json.Unmarshal([]byte(tt.wantJSON), &want))
				require.Equal(t, want, got)
			}

			if tt.username != "" {
				// A failed rotation must never be retried: a second call would
				// issue another password and invalidate the one just created.
				require.Equal(t, 1, rotateCalls)
			} else {
				require.Equal(t, 0, rotateCalls, "rotate should not be called when username is missing")
			}
		})
	}
}

func TestRunUsersRotatePasswordErrors(t *testing.T) {
	const taggedBody = `{"status":"sessions_not_terminated","error":"server-body-marker","password":"body-password-marker"}`
	flapsError := func(status int, body string) error {
		return &flaps.FlapsError{
			ResponseStatusCode: status,
			ResponseBody:       []byte(body),
			// SDK errors can retain server prose; generic handling deliberately preserves it.
			OriginalError: errors.New("server-body-marker: body-password-marker"),
		}
	}
	// Even if a server message suggests retrying or contains sensitive text,
	// the confirmed partial-success path must use only our fixed diagnostic.
	unsafeMessage := flapsError(500, taggedBody).(*flaps.FlapsError)
	unsafeMessage.OriginalError = errors.New("rotate again: body-password-marker")

	tests := []struct {
		name      string
		err       error
		confirmed bool
		clientErr bool
	}{
		{name: "tagged 500", err: flapsError(500, taggedBody), confirmed: true},
		{name: "tagged error ignores server prose", err: unsafeMessage, confirmed: true},
		{name: "wrapped tagged error", err: fmt.Errorf("rotate: %w", flapsError(500, taggedBody)), confirmed: true},
		{name: "tagged 503", err: flapsError(503, taggedBody), confirmed: true},
		{name: "tagged 599", err: flapsError(599, taggedBody), confirmed: true},
		{name: "exactly 64 KiB", err: flapsError(500, taggedBody+strings.Repeat(" ", 64*1024-len(taggedBody))), confirmed: true},
		{name: "other 5xx", err: flapsError(502, `{"error":"bad gateway"}`)},
		{name: "old code tag", err: flapsError(500, `{"code":"sessions_not_terminated"}`)},
		{name: "old server", err: flapsError(500, `{"error":"Password rotated, but we could not kill existing sessions."}`)},
		{name: "malformed", err: flapsError(500, `<html>server error</html>`)},
		{name: "empty", err: flapsError(500, "")},
		{name: "truncated", err: flapsError(500, `{"status":"sessions_not_terminated"`)},
		{name: "trailing garbage", err: flapsError(500, taggedBody+"garbage")},
		{name: "multiple objects", err: flapsError(500, taggedBody+`{}`)},
		{name: "nonstring status", err: flapsError(500, `{"status":123}`)},
		{name: "null status", err: flapsError(500, `{"status":null}`)},
		{name: "unknown status", err: flapsError(500, `{"status":"rotation_failed"}`)},
		{name: "status must match exactly", err: flapsError(500, `{"status":"sessions_not_terminated "}`)},
		{name: "status is case sensitive", err: flapsError(500, `{"status":"SESSIONS_NOT_TERMINATED"}`)},
		{name: "nested status", err: flapsError(500, `{"error":{"status":"sessions_not_terminated"}}`)},
		{name: "oversized body", err: flapsError(500, taggedBody+strings.Repeat(" ", 64*1024+1-len(taggedBody)))},
		{name: "tagged 400", err: flapsError(400, taggedBody), clientErr: true},
		{name: "wrapped tagged 422", err: fmt.Errorf("rotate: %w", flapsError(422, taggedBody)), clientErr: true},
		{name: "tagged 499", err: flapsError(499, taggedBody), clientErr: true},
		{name: "status outside 5xx", err: flapsError(600, taggedBody)},
		{name: "transport error", err: errors.New("connection reset")},
	}

	for _, tt := range tests {
		for _, jsonOutput := range []bool{false, true} {
			for _, killSessions := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/json=%t/kill-sessions=%t", tt.name, jsonOutput, killSessions), func(t *testing.T) {
					ctx, stdout, stderr := usersTestContext(t, jsonOutput)
					require.NoError(t, flag.FromContext(ctx).Set("yes", "true"))
					require.NoError(t, flag.SetString(ctx, "username", "app_user"))
					require.NoError(t, flag.FromContext(ctx).Set("kill-sessions", fmt.Sprint(killSessions)))
					rotateCalls, credentialCalls := 0, 0
					ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
						RotateManagedPostgresUserPasswordFunc: func(_ context.Context, id, username string, req flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
							rotateCalls++
							require.Equal(t, "mpg-123", id)
							require.Equal(t, "app_user", username)
							require.Equal(t, killSessions, req.KillSessions)
							return flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "returned-password-marker"}, tt.err
						},
						GetManagedPostgresUserCredentialsFunc: func(_ context.Context, id, username string) (flaps.ManagedPostgresUserCredentials, error) {
							require.Equal(t, "mpg-123", id)
							require.Equal(t, "app_user", username)
							credentialCalls++
							return flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "recovered-current-password"}, nil
						},
					})
					ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
						GetUserCredentialsFunc: func(context.Context, string, string) (mpgv2.GetUserCredentialsResponse, error) {
							credentialCalls++
							return mpgv2.GetUserCredentialsResponse{}, errors.New("unexpected legacy credential recovery")
						},
					})

					err := RunUsersRotatePassword(ctx, "mpg-123")
					require.Error(t, err, "partial success must still fail the command")
					require.Equal(t, 1, rotateCalls, "never retry a failed rotation")
					if tt.confirmed {
						require.Equal(t, 1, credentialCalls, "read current credentials once without rotating again")
					} else {
						require.Zero(t, credentialCalls, "only confirmed rotation permits recovery")
					}
					if tt.confirmed {
						require.Contains(t, stdout.String(), "recovered-current-password")
						require.NotContains(t, stdout.String(), "were terminated")
						if jsonOutput {
							var current flaps.ManagedPostgresUserCredentials
							require.NoError(t, json.Unmarshal(stdout.Bytes(), &current))
							require.Equal(t, flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "recovered-current-password"}, current)
						}
					} else if jsonOutput {
						require.Empty(t, stdout.String(), "no new JSON failure schema")
					} else {
						require.Equal(t, "Rotating password for user app_user in cluster mpg-123...\n", stdout.String())
					}
					if tt.confirmed {
						require.EqualError(t, err, "failed to terminate existing sessions after rotating password for user app_user")
						require.Equal(t, "Note: the password was rotated, but existing sessions could not be terminated. Existing sessions may still be active and need to be terminated manually.\n", stderr.String())
						require.NotContains(t, stderr.String()+err.Error(), "rotate again")
						for _, marker := range []string{"server-body-marker", "body-password-marker"} {
							require.NotContains(t, stdout.String()+stderr.String()+err.Error(), marker)
						}
					} else {
						require.ErrorIs(t, err, tt.err)
						require.ErrorContains(t, err, "failed to rotate password for user app_user")
						if tt.clientErr {
							require.Empty(t, stderr.String())
						} else {
							want := "Note: the password may already have been rotated even though this command failed. The server does not report rotation status atomically with errors. Check the current credentials in the dashboard Connect tab before considering another rotation.\n"
							if killSessions {
								want += "Note: with --kill-sessions, session termination is a separate step from rotation and may not have completed.\n"
							}
							require.Equal(t, want, stderr.String(), "preserve generic ambiguity behavior")
						}
					}
					require.NotContains(t, stdout.String()+stderr.String()+err.Error(), "returned-password-marker")
				})
			}
		}
	}
}

func TestRunUsersRotatePasswordRequiresConfirmation(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, killSessions := range []bool{false, true} {
			t.Run(fmt.Sprintf("json=%t/kill=%t", jsonOutput, killSessions), func(t *testing.T) {
				ctx, stdout, stderr := usersTestContext(t, jsonOutput)
				require.NoError(t, flag.SetString(ctx, "username", "app_user"))
				require.NoError(t, flag.FromContext(ctx).Set("kill-sessions", fmt.Sprint(killSessions)))
				calls := 0
				ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{RotateManagedPostgresUserPasswordFunc: func(context.Context, string, string, flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
					calls++
					return flaps.ManagedPostgresUserCredentials{}, nil
				}})
				err := RunUsersRotatePassword(ctx, "mpg-123")
				require.ErrorContains(t, err, "--yes flag must be specified")
				require.Zero(t, calls)
				require.Empty(t, stdout.String())
				require.Contains(t, stderr.String(), "DATABASE_URL")
				require.Contains(t, stderr.String(), "cannot be undone")
				require.Equal(t, killSessions, strings.Contains(stderr.String(), "Existing database sessions will also be terminated"))
			})
		}
	}
}

func TestRunUsersRotatePasswordRecoveryFailure(t *testing.T) {
	tests := []struct {
		name        string
		credentials flaps.ManagedPostgresUserCredentials
		err         error
	}{
		{"get error", flaps.ManagedPostgresUserCredentials{Password: "untrusted-get-password"}, errors.New("private-get-error")},
		{"mismatched username", flaps.ManagedPostgresUserCredentials{Username: "other_user", Password: "untrusted-get-password"}, nil},
		{"empty password", flaps.ManagedPostgresUserCredentials{Username: "app_user"}, nil},
	}
	for _, tt := range tests {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%t", tt.name, jsonOutput), func(t *testing.T) {
				ctx, stdout, stderr := usersTestContext(t, jsonOutput)
				require.NoError(t, flag.SetString(ctx, "username", "app_user"))
				require.NoError(t, flag.FromContext(ctx).Set("yes", "true"))
				rotations, reads := 0, 0
				ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
					RotateManagedPostgresUserPasswordFunc: func(context.Context, string, string, flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
						rotations++
						return flaps.ManagedPostgresUserCredentials{Password: "untrusted-rotation-password"}, &flaps.FlapsError{ResponseStatusCode: 500, ResponseBody: []byte(`{"status":"sessions_not_terminated"}`), OriginalError: errors.New("private-rotation-error")}
					},
					GetManagedPostgresUserCredentialsFunc: func(_ context.Context, clusterID, username string) (flaps.ManagedPostgresUserCredentials, error) {
						reads++
						require.Equal(t, "mpg-123", clusterID)
						require.Equal(t, "app_user", username)
						return tt.credentials, tt.err
					},
				})
				err := RunUsersRotatePassword(ctx, "mpg-123")
				require.ErrorContains(t, err, "dashboard Connect tab")
				require.ErrorContains(t, err, "do not retry rotation")
				require.Equal(t, 1, rotations)
				require.Equal(t, 1, reads)
				for _, marker := range []string{"untrusted-rotation-password", "untrusted-get-password", "private-rotation-error", "private-get-error"} {
					require.NotContains(t, stdout.String()+stderr.String()+err.Error(), marker)
				}
				if jsonOutput {
					require.Empty(t, stdout.String())
				}
			})
		}
	}
}

type credentialOutputFailure struct{}

func (credentialOutputFailure) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }
func TestRenderRotatedUserCredentialsOutputFailure(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		err := renderRotatedUserCredentials(credentialOutputFailure{}, flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "sensitive-password"}, jsonOutput, false, true)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "sensitive-password")
	}
}

func TestRunUsersRotatePasswordJSONDoesNotPrompt(t *testing.T) {
	for _, username := range []string{"", "app_user"} {
		ctx, stdout, _ := usersTestContext(t, true)
		io := iostreams.FromContext(ctx)
		io.SetStdinTTY(true)
		io.SetStdoutTTY(true)
		require.NoError(t, flag.SetString(ctx, "username", username))
		calls := 0
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{RotateManagedPostgresUserPasswordFunc: func(context.Context, string, string, flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
			calls++
			return flaps.ManagedPostgresUserCredentials{}, nil
		}})
		err := RunUsersRotatePassword(ctx, "mpg-123")
		require.ErrorContains(t, err, "flag")
		require.Zero(t, calls)
		require.Empty(t, stdout.String())
	}
}

func TestRunUsersRotatePasswordRecoveryOutputFailure(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		ctx, _, stderr := usersTestContext(t, jsonOutput)
		iostreams.FromContext(ctx).Out = credentialOutputFailure{}
		require.NoError(t, flag.SetString(ctx, "username", "app_user"))
		require.NoError(t, flag.FromContext(ctx).Set("yes", "true"))
		mutations, reads := 0, 0
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			RotateManagedPostgresUserPasswordFunc: func(context.Context, string, string, flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
				mutations++
				return flaps.ManagedPostgresUserCredentials{}, &flaps.FlapsError{ResponseStatusCode: 500, ResponseBody: []byte(`{"status":"sessions_not_terminated"}`)}
			},
			GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
				reads++
				return flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "sensitive-current-password"}, nil
			},
		})
		err := RunUsersRotatePassword(ctx, "mpg-123")
		require.ErrorContains(t, err, "current credentials could not be displayed")
		require.Equal(t, 1, mutations)
		require.Equal(t, 1, reads)
		require.NotContains(t, err.Error()+stderr.String(), "sensitive-current-password")
	}
}
