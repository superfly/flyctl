package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func databasesTestContext(t *testing.T, jsonOutput bool) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("name", "", "")

	return flagctx.NewContext(ctx, flags), stdout
}

func TestRunDatabasesList(t *testing.T) {
	publicDatabases := []flaps.ManagedPostgresDatabase{
		{Name: "app"},
		{Name: "metrics"},
	}
	legacyDatabases := []mpgv2.Database{
		{Name: "app"},
		{Name: "metrics"},
	}

	tests := []struct {
		name                string
		jsonOutput          bool
		publicDatabases     []flaps.ManagedPostgresDatabase
		publicListErr       error
		legacyDatabases     []mpgv2.Database
		legacyListErr       error
		wantPublicCalled    bool
		wantLegacyCalled    bool
		wantErr             string
		wantOutputContains  []string
		wantJSONOutputShape string
	}{
		{
			name:               "uses Machines API",
			publicDatabases:    publicDatabases,
			wantPublicCalled:   true,
			wantOutputContains: []string{"app", "metrics", "NAME"},
		},
		{
			name:                "renders JSON output from Machines API",
			jsonOutput:          true,
			publicDatabases:     publicDatabases,
			wantPublicCalled:    true,
			wantJSONOutputShape: `[{"name":"app"},{"name":"metrics"}]`,
		},
		{
			name:               "renders empty message when Machines API returns no databases",
			publicDatabases:    nil,
			wantPublicCalled:   true,
			wantOutputContains: []string{"No databases found for cluster mpg-123"},
		},
		{
			name:       "renders legacy JSON after wrapped Machines 404",
			jsonOutput: true,
			publicListErr: fmt.Errorf("list Managed Postgres databases: %w", &flaps.FlapsError{
				ResponseStatusCode: 404,
				OriginalError:      errors.New("not found"),
			}),
			legacyDatabases:     legacyDatabases,
			wantPublicCalled:    true,
			wantLegacyCalled:    true,
			wantJSONOutputShape: `[{"name":"app"},{"name":"metrics"}]`,
		},
		{
			name:             "returns non-404 Machines API error without falling back",
			publicListErr:    errors.New("boom"),
			wantPublicCalled: true,
			wantErr:          "failed to list databases for cluster mpg-123: boom",
		},
		{
			name:             "returns legacy error when fallback fails",
			publicListErr:    flaps.ErrFlapsNotFound,
			legacyListErr:    errors.New("legacy boom"),
			wantPublicCalled: true,
			wantLegacyCalled: true,
			wantErr:          "failed to list databases for cluster mpg-123: legacy boom",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := databasesTestContext(t, test.jsonOutput)
			publicCalled := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListManagedPostgresDatabasesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresDatabase, error) {
					publicCalled = true
					require.Equal(t, "mpg-123", id)

					return test.publicDatabases, test.publicListErr
				},
			})

			legacyCalled := false
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				ListDatabasesFunc: func(_ context.Context, id string) (mpgv2.ListDatabasesResponse, error) {
					legacyCalled = true
					require.Equal(t, "mpg-123", id)

					return mpgv2.ListDatabasesResponse{Data: test.legacyDatabases}, test.legacyListErr
				},
			})

			err := RunDatabasesList(ctx, "mpg-123")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantPublicCalled, publicCalled)
			require.Equal(t, test.wantLegacyCalled, legacyCalled)

			if test.wantJSONOutputShape != "" {
				var got []map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
				var want []map[string]any
				require.NoError(t, json.Unmarshal([]byte(test.wantJSONOutputShape), &want))
				require.Equal(t, want, got)
			}
			for _, want := range test.wantOutputContains {
				require.Contains(t, stdout.String(), want)
			}
		})
	}
}

func TestRunDatabasesCreate(t *testing.T) {
	publicCreated := flaps.ManagedPostgresDatabase{Name: "reports-created"}

	tests := []struct {
		name                string
		nameFlag            string
		publicCreated       flaps.ManagedPostgresDatabase
		publicCreateErr     error
		legacyCreateErr     error
		wantPublicCalled    bool
		wantLegacyCalled    bool
		wantPublicNameField string
		wantErr             string
		wantOutputContains  []string
	}{
		{
			name:                "uses Machines API",
			nameFlag:            "reports",
			publicCreated:       publicCreated,
			wantPublicCalled:    true,
			wantPublicNameField: "reports",
			wantOutputContains:  []string{"Creating database reports in cluster mpg-123...", "Database created successfully!", "Name: reports-created"},
		},
		{
			name:               "falls back to legacy API on public not found",
			nameFlag:           "reports",
			publicCreateErr:    flaps.ErrFlapsNotFound,
			wantPublicCalled:   true,
			wantLegacyCalled:   true,
			wantOutputContains: []string{"Creating database reports in cluster mpg-123...", "Database created successfully!", "Name: reports"},
		},
		{
			name:             "returns non-404 Machines API error without falling back",
			nameFlag:         "reports",
			publicCreateErr:  errors.New("boom"),
			wantPublicCalled: true,
			wantErr:          "failed to create database: boom",
		},
		{
			name:             "returns legacy error when fallback fails",
			nameFlag:         "reports",
			publicCreateErr:  flaps.ErrFlapsNotFound,
			legacyCreateErr:  errors.New("legacy boom"),
			wantPublicCalled: true,
			wantLegacyCalled: true,
			wantErr:          "failed to create database: legacy boom",
		},
		{
			name:    "requires --name when not interactive",
			wantErr: "database name must be specified with --name flag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := databasesTestContext(t, false)
			if test.nameFlag != "" {
				require.NoError(t, flag.SetString(ctx, "name", test.nameFlag))
			}

			publicCalled := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				CreateManagedPostgresDatabaseFunc: func(_ context.Context, id string, req flaps.CreateManagedPostgresDatabaseRequest) (flaps.ManagedPostgresDatabase, error) {
					publicCalled = true
					require.Equal(t, "mpg-123", id)
					if test.wantPublicNameField != "" {
						require.Equal(t, test.wantPublicNameField, req.Name)
					}

					return test.publicCreated, test.publicCreateErr
				},
			})

			legacyCalled := false
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				CreateDatabaseFunc: func(_ context.Context, id string, input mpgv2.CreateDatabaseInput) error {
					legacyCalled = true
					require.Equal(t, "mpg-123", id)
					if test.nameFlag != "" {
						require.Equal(t, test.nameFlag, input.Name)
					}

					return test.legacyCreateErr
				},
			})

			err := RunDatabasesCreate(ctx, "mpg-123")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantPublicCalled, publicCalled)
			require.Equal(t, test.wantLegacyCalled, legacyCalled)
			for _, want := range test.wantOutputContains {
				require.Contains(t, stdout.String(), want)
			}
		})
	}
}
