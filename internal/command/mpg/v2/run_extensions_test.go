package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
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
	tests := []struct {
		name             string
		jsonOutput       bool
		nullMetadata     bool
		publicExtensions []flaps.ManagedPostgresExtension
		publicErr        error
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
			err := RunExtensionsList(ctx, "mpg-123", "app")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, publicCalls)

			if test.jsonOutput {
				var got []map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
				if test.nullMetadata {
					require.Equal(t, []map[string]any{{
						"name": "hstore", "description": "", "docs_url": "", "default_version": "", "is_system": false, "installed": nil,
					}}, got)

					return
				}
				require.Equal(t, []map[string]any{
					{"name": "pg_trgm", "description": "text similarity", "docs_url": "", "default_version": "1.6", "is_system": false, "installed": nil},
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
		name          string
		extension     string
		schema        string
		createSchema  bool
		publicErr     error
		wantPublicReq flaps.EnableManagedPostgresExtensionRequest
		wantErr       string
	}{
		{name: "maps public request options", extension: "hstore", schema: "addons", createSchema: true, wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "hstore", Schema: "addons", CreateSchema: true}},
		{name: "defaults postgis topology schema", extension: "postgis_topology", wantPublicReq: flaps.EnableManagedPostgresExtensionRequest{Name: "postgis_topology", Schema: "topology", CreateSchema: true}},
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
			err := RunExtensionsEnable(ctx, "mpg-123", "app", test.extension, test.schema, test.createSchema)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "Extension "+test.extension+" enabled on database app.")
			}
			require.Equal(t, 1, publicCalls)
		})
	}
}

func TestRunExtensionsDisable(t *testing.T) {
	tests := []struct {
		name      string
		force     bool
		publicErr error
		wantErr   string
	}{
		{name: "public success maps force", force: true},
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
			err := RunExtensionsDisable(ctx, "mpg-123", "app", "hstore", test.force)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), "Extension hstore disabled on database app.")
			}
			require.Equal(t, 1, publicCalls)
		})
	}
}

func TestResolveDatabaseUsesPublicAPI(t *testing.T) {
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

func TestRunExtensionsListReturnsPublic404(t *testing.T) {
	ctx, _ := extensionsTestContext(t, false)
	publicErr := &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("cluster not found")}
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListManagedPostgresExtensionsFunc: func(context.Context, string, string) ([]flaps.ManagedPostgresExtension, error) {
			return nil, publicErr
		},
	})
	err := RunExtensionsList(ctx, "mpg-123", "app")
	require.ErrorIs(t, err, publicErr)
	require.ErrorContains(t, err, "failed to list extensions for database app: cluster not found")
}
