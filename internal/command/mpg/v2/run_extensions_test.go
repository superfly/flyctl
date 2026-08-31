package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func extensionsTestContext(t *testing.T, jsonOutput bool) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})

	return ctx, stdout
}

func TestRunExtensionsList(t *testing.T) {
	publicExtensions := []flaps.ManagedPostgresExtension{
		{Name: "pg_trgm", Description: stringPointer("text similarity"), DefaultVersion: stringPointer("1.6")},
		{Name: "plpgsql", Description: stringPointer("PL/pgSQL"), DefaultVersion: stringPointer("1.0"), System: true, Installed: &flaps.ManagedPostgresInstalledExtension{Version: "1.0", Schema: "pg_catalog"}},
	}
	nullMetadataExtensions := []flaps.ManagedPostgresExtension{{Name: "hstore"}}
	legacyExtensions := []mpgv2.Extension{
		{Name: "pg_trgm", Description: "text similarity", DocsURL: "https://example.test/pg_trgm", DefaultVersion: "1.6"},
		{Name: "plpgsql", Description: "PL/pgSQL", DefaultVersion: "1.0", IsSystem: true, Installed: &mpgv2.InstalledExtension{Version: "1.0", Schema: "pg_catalog"}},
	}

	tests := []struct {
		name             string
		jsonOutput       bool
		nullMetadata     bool
		publicExtensions []flaps.ManagedPostgresExtension
		publicErr        error
		legacyExtensions []mpgv2.Extension
		legacyErr        error
		wantLegacyCalls  int
		wantErr          string
		wantOutput       []string
	}{
		{
			name:             "public success renders installed and uninstalled table rows",
			publicExtensions: publicExtensions,
			wantOutput:       []string{"Extensions in database app", "pg_trgm", "no", "plpgsql", "yes", "1.0", "pg_catalog", "PL/pgSQL"},
		},
		{
			name:             "public success preserves legacy JSON shape",
			jsonOutput:       true,
			publicExtensions: publicExtensions,
		},
		{
			name:             "public null metadata renders blank table fields",
			publicExtensions: nullMetadataExtensions,
			wantOutput:       []string{"Extensions in database app", "hstore", "no"},
		},
		{
			name:             "public null metadata preserves legacy JSON shape",
			jsonOutput:       true,
			nullMetadata:     true,
			publicExtensions: nullMetadataExtensions,
		},
		{
			name:             "classified 404 falls back",
			publicErr:        fmt.Errorf("wrapped: %w", flaps.ErrFlapsNotFound),
			legacyExtensions: legacyExtensions,
			wantLegacyCalls:  1,
			wantOutput:       []string{"pg_trgm", "plpgsql", "yes", "pg_catalog"},
		},
		{
			name:             "classified 404 preserves legacy JSON fields",
			jsonOutput:       true,
			publicErr:        flaps.ErrFlapsNotFound,
			legacyExtensions: legacyExtensions,
			wantLegacyCalls:  1,
		},
		{
			name:            "fallback preserves legacy error",
			publicErr:       flaps.ErrFlapsNotFound,
			legacyErr:       errors.New("legacy denied"),
			wantLegacyCalls: 1,
			wantErr:         "failed to list extensions for database app: legacy denied",
		},
		{
			name:      "non-404 public error is authoritative",
			publicErr: errors.New("public unavailable"),
			wantErr:   "failed to list extensions for database app: public unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := extensionsTestContext(t, test.jsonOutput)
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListManagedPostgresExtensionsFunc: func(_ context.Context, id, database string) ([]flaps.ManagedPostgresExtension, error) {
					publicCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "app", database)

					return test.publicExtensions, test.publicErr
				},
			})
			legacyCalls := 0
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				ListExtensionsFunc: func(_ context.Context, id, database string) (mpgv2.ListExtensionsResponse, error) {
					legacyCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "app", database)

					return mpgv2.ListExtensionsResponse{Data: test.legacyExtensions}, test.legacyErr
				},
			})

			err := RunExtensionsList(ctx, "mpg-123", "app")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, test.wantLegacyCalls, legacyCalls)

			if test.jsonOutput {
				var got []map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
				if test.nullMetadata {
					require.Equal(t, []map[string]any{{
						"name": "hstore", "description": "", "docs_url": "", "default_version": "", "is_system": false, "installed": nil,
					}}, got)
					return
				}
				docsURL := ""
				if test.wantLegacyCalls == 1 {
					docsURL = "https://example.test/pg_trgm"
				}
				require.Equal(t, []map[string]any{
					{"name": "pg_trgm", "description": "text similarity", "docs_url": docsURL, "default_version": "1.6", "is_system": false, "installed": nil},
					{"name": "plpgsql", "description": "PL/pgSQL", "docs_url": "", "default_version": "1.0", "is_system": true, "installed": map[string]any{"version": "1.0", "schema": "pg_catalog"}},
				}, got)
			}
			for _, want := range test.wantOutput {
				require.Contains(t, stdout.String(), want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestRunExtensionsEnable(t *testing.T) {
	tests := []struct {
		name            string
		extension       string
		schema          string
		createSchema    bool
		publicErr       error
		legacyErr       error
		wantPublicReq   flaps.EnableManagedPostgresExtensionRequest
		wantLegacyCalls int
		wantErr         string
	}{
		{name: "maps public request options", extension: "hstore", schema: "addons", createSchema: true, wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "hstore", Schema: "addons", CreateSchema: true}},
		{name: "defaults postgis topology schema", extension: "postgis_topology", wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "postgis_topology", Schema: "topology", CreateSchema: true}},
		{name: "classified 404 falls back with legacy request mapping", extension: "hstore", schema: "addons", createSchema: true, publicErr: flaps.ErrFlapsNotFound, wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "hstore", Schema: "addons", CreateSchema: true}, wantLegacyCalls: 1},
		{name: "fallback returns legacy error", extension: "hstore", publicErr: flaps.ErrFlapsNotFound, legacyErr: errors.New("legacy denied"), wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "hstore"}, wantLegacyCalls: 1, wantErr: "legacy denied"},
		{name: "non-404 public error is authoritative", extension: "hstore", publicErr: errors.New("public denied"), wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "hstore"}, wantErr: "public denied"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := extensionsTestContext(t, false)
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				EnableManagedPostgresExtensionFunc: func(_ context.Context, id, database string, req flaps.EnableManagedPostgresExtensionRequest) error {
					publicCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "app", database)
					require.Equal(t, test.wantPublicReq, req)

					return test.publicErr
				},
			})
			legacyCalls := 0
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				EnableExtensionFunc: func(_ context.Context, id, database string, input mpgv2.EnableExtensionInput) error {
					legacyCalls++
					require.Equal(t, test.wantPublicReq.Name, input.Name)
					require.Equal(t, test.wantPublicReq.Schema, input.Schema)
					require.Equal(t, test.wantPublicReq.CreateSchema, input.CreateSchemaIfNeeded)

					return test.legacyErr
				},
			})

			err := RunExtensionsEnable(ctx, "mpg-123", "app", test.extension, test.schema, test.createSchema)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "Extension "+test.extension+" enabled on database app.")
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, test.wantLegacyCalls, legacyCalls)
		})
	}
}

func TestRunExtensionsDisable(t *testing.T) {
	tests := []struct {
		name            string
		force           bool
		publicErr       error
		legacyErr       error
		wantLegacyCalls int
		wantErr         string
	}{
		{name: "public success maps force", force: true},
		{name: "classified 404 falls back", force: true, publicErr: flaps.ErrFlapsNotFound, wantLegacyCalls: 1},
		{name: "fallback returns legacy error", publicErr: flaps.ErrFlapsNotFound, legacyErr: errors.New("legacy denied"), wantLegacyCalls: 1, wantErr: "legacy denied"},
		{name: "non-404 public error is authoritative", publicErr: errors.New("public denied"), wantErr: "public denied"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := extensionsTestContext(t, false)
			publicCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				DisableManagedPostgresExtensionFunc: func(_ context.Context, id, database, name string, force bool) error {
					publicCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "app", database)
					require.Equal(t, "hstore", name)
					require.Equal(t, test.force, force)

					return test.publicErr
				},
			})
			legacyCalls := 0
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				DisableExtensionFunc: func(_ context.Context, id, database, name string, force bool) error {
					legacyCalls++
					require.Equal(t, test.force, force)

					return test.legacyErr
				},
			})

			err := RunExtensionsDisable(ctx, "mpg-123", "app", "hstore", test.force)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "Extension hstore disabled on database app.")
			}
			require.Equal(t, 1, publicCalls)
			require.Equal(t, test.wantLegacyCalls, legacyCalls)
		})
	}
}

func TestResolveDatabaseUsesPublicAPIWithLegacyFallback(t *testing.T) {
	t.Run("explicit database skips resolution", func(t *testing.T) {
		ctx, _ := extensionsTestContext(t, false)
		database, err := resolveDatabase(ctx, "mpg-123", "explicit-db")
		require.NoError(t, err)
		require.Equal(t, "explicit-db", database)
	})

	t.Run("public success", func(t *testing.T) {
		ctx, _ := extensionsTestContext(t, false)
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			ListManagedPostgresDatabasesFunc: func(_ context.Context, id string) ([]flaps.ManagedPostgresDatabase, error) {
				require.Equal(t, "mpg-123", id)
				return []flaps.ManagedPostgresDatabase{{Name: "only-db"}}, nil
			},
		})
		database, err := resolveDatabase(ctx, "mpg-123", "")
		require.NoError(t, err)
		require.Equal(t, "only-db", database)
	})

	t.Run("classified 404 falls back", func(t *testing.T) {
		ctx, _ := extensionsTestContext(t, false)
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			ListManagedPostgresDatabasesFunc: func(context.Context, string) ([]flaps.ManagedPostgresDatabase, error) {
				return nil, flaps.ErrFlapsNotFound
			},
		})
		legacyCalls := 0
		ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
			ListDatabasesFunc: func(_ context.Context, id string) (mpgv2.ListDatabasesResponse, error) {
				legacyCalls++
				require.Equal(t, "mpg-123", id)
				return mpgv2.ListDatabasesResponse{Data: []mpgv2.Database{{Name: "legacy-db"}}}, nil
			},
		})
		database, err := resolveDatabase(ctx, "mpg-123", "")
		require.NoError(t, err)
		require.Equal(t, "legacy-db", database)
		require.Equal(t, 1, legacyCalls)
	})

	t.Run("non-404 public error is authoritative", func(t *testing.T) {
		ctx, _ := extensionsTestContext(t, false)
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			ListManagedPostgresDatabasesFunc: func(context.Context, string) ([]flaps.ManagedPostgresDatabase, error) {
				return nil, errors.New("public denied")
			},
		})
		_, err := resolveDatabase(ctx, "mpg-123", "")
		require.ErrorContains(t, err, "failed to list databases: public denied")
	})
}
