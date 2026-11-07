package cmdv2

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func rolesNotAvailable() error {
	return &flaps.FlapsError{
		ResponseStatusCode: 501,
		OriginalError:      errors.New("Customer roles are not available on this cluster yet"),
	}
}

func TestRunRolesList(t *testing.T) {
	for _, tt := range []struct {
		name       string
		jsonOutput bool
		roles      []flaps.ManagedPostgresRole
		err        error
		wantRows   [][]string
		wantOut    string
		wantJSON   string
		wantErr    string
		wantHint   bool
	}{
		{
			name:  "table",
			roles: []flaps.ManagedPostgresRole{{Name: "Team B", MemberCount: 1}, {Name: "analytics", MemberCount: 3}, {Name: "etl"}},
			wantRows: [][]string{
				{"NAME", "MEMBERS"},
				{"Team B", "1"},
				{"analytics", "3"},
				{"etl", "0"},
			},
		},
		{name: "json", jsonOutput: true, roles: []flaps.ManagedPostgresRole{{Name: "analytics", MemberCount: 3}}, wantJSON: `[{"name":"analytics","member_count":3}]`},
		{name: "empty", roles: []flaps.ManagedPostgresRole{}, wantOut: "No roles found in cluster mpg-123\n"},
		{name: "empty json", jsonOutput: true, roles: []flaps.ManagedPostgresRole{}, wantJSON: `[]`},
		{name: "old cluster gets the update hint", err: rolesNotAvailable(), wantErr: "failed to list roles for cluster mpg-123: Customer roles are not available on this cluster yet", wantHint: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, tt.jsonOutput)
			calls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{ListManagedPostgresRolesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresRole, error) {
				calls++
				require.Equal(t, "mpg-123", id)

				return tt.roles, tt.err
			}})

			err := RunRolesList(ctx, "mpg-123")
			require.Equal(t, 1, calls)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantHint {
				requireClusterUpdateHint(t, err)
			}
			if tt.wantRows != nil {
				require.Equal(t, tt.wantRows, tableCells(stdout.String()))
			}
			if tt.wantOut != "" {
				require.Equal(t, tt.wantOut, stdout.String())
			}
			if tt.wantJSON != "" {
				require.JSONEq(t, tt.wantJSON, stdout.String())
			}
		})
	}
}

func TestRunRolesMembers(t *testing.T) {
	for _, tt := range []struct {
		name       string
		jsonOutput bool
		role       string
		members    []flaps.ManagedPostgresRoleMember
		err        error
		wantCall   bool
		wantRows   [][]string
		wantOut    string
		wantJSON   string
		wantErr    string
		wantHint   bool
	}{
		{
			name:     "table, role sent verbatim",
			role:     "Mixed Case/role",
			members:  []flaps.ManagedPostgresRoleMember{{Name: "analyst"}, {Name: "writer"}},
			wantCall: true,
			wantRows: [][]string{{"NAME"}, {"analyst"}, {"writer"}},
		},
		{name: "json", jsonOutput: true, role: "analytics", members: []flaps.ManagedPostgresRoleMember{{Name: "analyst"}}, wantCall: true, wantJSON: `[{"name":"analyst"}]`},
		{name: "empty", role: "analytics", members: []flaps.ManagedPostgresRoleMember{}, wantCall: true, wantOut: "Role analytics has no members\n"},
		{name: "empty json", jsonOutput: true, role: "analytics", members: []flaps.ManagedPostgresRoleMember{}, wantCall: true, wantJSON: `[]`},
		{name: "role required when not interactive", wantErr: "role must be specified with --role flag when not running interactively"},
		{name: "role required with json", jsonOutput: true, wantErr: "role must be specified with --role flag when using --json"},
		{name: "missing role", role: "gone", err: flaps.ErrFlapsNotFound, wantCall: true, wantErr: "failed to list members of role gone in cluster mpg-123"},
		{name: "old cluster gets the update hint", role: "analytics", err: rolesNotAvailable(), wantCall: true, wantErr: "failed to list members of role analytics in cluster mpg-123", wantHint: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stdout, _ := usersTestContext(t, tt.jsonOutput)
			if tt.role != "" {
				require.NoError(t, flag.SetString(ctx, "role", tt.role))
			}
			calls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{ListManagedPostgresRoleMembersFunc: func(_ context.Context, id, role string) ([]flaps.ManagedPostgresRoleMember, error) {
				calls++
				require.Equal(t, "mpg-123", id)
				require.Equal(t, tt.role, role)

				return tt.members, tt.err
			}})

			err := RunRolesMembers(ctx, "mpg-123")
			require.Equal(t, tt.wantCall, calls == 1)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, stdout.String())
			} else {
				require.NoError(t, err)
			}
			if tt.wantHint {
				requireClusterUpdateHint(t, err)
			}
			if tt.wantRows != nil {
				require.Equal(t, tt.wantRows, tableCells(stdout.String()))
			}
			if tt.wantOut != "" {
				require.Equal(t, tt.wantOut, stdout.String())
			}
			if tt.wantJSON != "" {
				require.JSONEq(t, tt.wantJSON, stdout.String())
			}
		})
	}
}
