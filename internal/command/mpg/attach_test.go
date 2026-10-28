package mpg

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunAttachV1DirectVariableFlag(t *testing.T) {
	for _, value := range []string{"omitted", "no-direct-url", "DIRECT_URL", "MIGRATION_URL", ""} {
		t.Run(value, func(t *testing.T) {
			ctx := appconfig.WithName(setupTestContext(), "my-app")
			flags := pflag.NewFlagSet("attach", pflag.ContinueOnError)
			flags.String("direct-variable-name", "DIRECT_URL", "")
			flags.Bool("no-direct-url", false, "")
			flags.String("username", "", "")
			flags.String("database", "", "")
			if value == "no-direct-url" {
				require.NoError(t, flags.Set("no-direct-url", "true"))
			} else if value != "omitted" {
				require.NoError(t, flags.Set("direct-variable-name", value))
			}
			require.NoError(t, flags.Parse([]string{"mpg-v1"}))
			ctx = flagctx.NewContext(ctx, flags)
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) {
					return &flaps.App{Organization: flaps.AppOrganizationInfo{Slug: "my-org"}}, nil
				},
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound
				},
			})
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
					return &uiex.Organization{Slug: "my-org", RawSlug: "my-org"}, nil
				},
			})
			calls := 0
			v1AttachErr := errors.New("v1 attach reached")
			ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
				GetManagedClusterByIdFunc: func(context.Context, string) (mpgv1.GetManagedClusterResponse, error) {
					calls++
					if calls > 1 {
						return mpgv1.GetManagedClusterResponse{}, v1AttachErr
					}
					return mpgv1.GetManagedClusterResponse{Data: mpgv1.ManagedCluster{
						Id: "mpg-v1", Version: 1, Organization: fly.Organization{Slug: "my-org"},
					}}, nil
				},
			})
			err := runAttach(ctx)
			if value == "omitted" || value == "no-direct-url" {
				require.ErrorIs(t, err, v1AttachErr)
				require.Equal(t, 2, calls)
			} else {
				require.EqualError(t, err, "--direct-variable-name is only supported for v2 clusters")
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestRunAttachV2DirectVariableFlagReachesV2(t *testing.T) {
	ctx := appconfig.WithName(setupTestContext(), "my-app")
	cmd := newAttach()
	require.NoError(t, cmd.ParseFlags([]string{"--direct-variable-name", "MIGRATION_URL", "mpg-v2"}))
	value, err := cmd.Flags().GetString("direct-variable-name")
	require.NoError(t, err)
	require.Equal(t, "MIGRATION_URL", value)
	ctx = flagctx.NewContext(ctx, cmd.Flags())
	calls := 0
	sentinel := errors.New("v2 attach reached")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(context.Context, string) (*flaps.App, error) {
			return &flaps.App{Organization: flaps.AppOrganizationInfo{Slug: "my-org"}}, nil
		},
		GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
			calls++
			if calls > 1 {
				return flaps.ManagedPostgresCluster{}, sentinel
			}
			return flaps.ManagedPostgresCluster{ID: "mpg-v2", Organization: flaps.ManagedPostgresOrganization{Slug: "my-org"}}, nil
		},
	})
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
			return &uiex.Organization{Slug: "my-org", RawSlug: "my-org"}, nil
		},
	})
	client := flapsutil.ClientFromContext(ctx).(*mock.FlapsClient)
	client.ListAppSecretsFunc = func(context.Context, string, *uint64, bool) ([]fly.AppSecret, error) { return nil, nil }
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{})
	require.ErrorIs(t, runAttach(ctx), sentinel)
	require.Equal(t, 2, calls)
}

func TestRunAttach_secretNamesBeforePrompts(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		equal    bool
		conflict bool
	}{
		{name: "defaults"},
		{name: "conflicting direct flags", args: []string{"--no-direct-url", "--direct-variable-name", "MIGRATION_URL"}, conflict: true},
		{name: "conflicting empty direct name", args: []string{"--no-direct-url", "--direct-variable-name="}, conflict: true},
		{name: "custom direct", args: []string{"--direct-variable-name", "MIGRATION_URL"}},
		{name: "pooled uses default direct name", args: []string{"--variable-name", "DIRECT_URL"}},
		{name: "custom pooled", args: []string{"--variable-name", "ANALYTICS_URL"}},
		{name: "equal default pooled", args: []string{"--direct-variable-name", "DATABASE_URL"}, equal: true},
		{name: "equal custom pooled", args: []string{"--variable-name", "SAME_URL", "--direct-variable-name", "SAME_URL"}, equal: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext()
			calls := 0
			sentinel := errors.New("app lookup reached")
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) {
					calls++
					return nil, sentinel
				},
			})
			cmd := newAttach()
			require.Equal(t, "false", cmd.Flags().Lookup("no-direct-url").DefValue)
			require.Equal(t, "DIRECT_URL", cmd.Flags().Lookup("direct-variable-name").DefValue)
			require.Equal(t, "DATABASE_URL", cmd.Flags().Lookup("variable-name").DefValue)
			require.NoError(t, cmd.ParseFlags(tt.args))
			ctx = flagctx.NewContext(ctx, cmd.Flags())
			io := iostreams.FromContext(ctx)
			io.SetStdinTTY(true)
			io.SetStdoutTTY(true)
			io.SetStderrTTY(true)
			err := runAttach(ctx)
			if tt.conflict {
				var conflict flyerr.GenericErr
				require.ErrorAs(t, err, &conflict)
				require.Equal(t, "Cannot specify both --direct-variable-name and --no-direct-url", conflict.Err)
				require.Equal(t, "Remove either --direct-variable-name or --no-direct-url", conflict.Suggest)
				require.Zero(t, calls)
			} else if tt.equal {
				require.EqualError(t, err, "pooled and direct variable names must be different")
				require.Zero(t, calls)
			} else {
				require.ErrorIs(t, err, sentinel)
				require.Equal(t, 1, calls)
			}
		})
	}
}
