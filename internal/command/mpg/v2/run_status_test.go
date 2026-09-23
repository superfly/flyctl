package cmdv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func statusTestContext(t *testing.T, jsonOutput bool) (context.Context, *bytes.Buffer) {
	t.Helper()
	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)

	return iostreams.NewContext(config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput}), io), stdout
}

// columnLineRegex matches a `<col> │ <value>` row anchored on the header so
// "10" (disk) and "10.0.0.1" (Direct IP) cannot satisfy each other.
func columnLineRegex(col, value string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(?m)^\s*%s\s*│\s*%s\s*$`, regexp.QuoteMeta(col), regexp.QuoteMeta(value)))
}

// samplePublicCluster is a representative public Machines API cluster payload.
func samplePublicCluster() flaps.ManagedPostgresCluster {
	used := int64(1288490188)
	provisioned := int64(21474836480)

	return flaps.ManagedPostgresCluster{
		ID:                      "mpg-123",
		Name:                    "test-cluster",
		Status:                  "ready",
		Region:                  "ord",
		Plan:                    "development",
		DiskSizeGB:              10,
		StorageUsedBytes:        &used,
		StorageProvisionedBytes: &provisioned,
		Replicas:                1,
		Organization:            flaps.ManagedPostgresOrganization{Name: "Test Org", Slug: "test-org"},
		Endpoints: flaps.ManagedPostgresEndpoints{
			Primary: struct {
				Direct flaps.ManagedPostgresEndpoint `json:"direct"`
				Pooler flaps.ManagedPostgresEndpoint `json:"pooler"`
			}{
				Direct: flaps.ManagedPostgresEndpoint{Host: "10.0.0.1", Port: 5432},
				Pooler: flaps.ManagedPostgresEndpoint{Host: "10.0.0.2", Port: 6432},
			},
		},
	}
}

// sampleLegacyCluster is a representative legacy MPGv2 cluster payload.
// Direct is a bare address (no port) — the historical shape of this column.
func sampleLegacyCluster() mpgv2.GetClusterResponse {
	used := int64(1288490188)
	provisioned := int64(21474836480)

	return mpgv2.GetClusterResponse{
		Data: mpgv2.ManagedCluster{
			Id: "mpg-123", Name: "test-cluster", Region: "ord", Status: "ready",
			Plan: "development", Disk: 10, Replicas: 1,
			StorageUsedBytes: &used, StorageProvisionedBytes: &provisioned,
			Organization:  fly.Organization{Name: "Test Org", Slug: "test-org"},
			IpAssignments: mpg.ManagedClusterIpAssignments{Direct: "10.0.0.1"},
		},
		Credentials: mpgv2.GetClusterCredentialsResponse{
			Status: "ready", User: "app", Password: "secret",
			DBName: "app", ConnectionUri: "postgres://app:secret@10.0.0.1:5432/app",
		},
	}
}

func TestRunStatusHuman(t *testing.T) {
	// IPv4 port exclusion proves JoinHostPort is not leaking ports; IPv6 test omitted.
	wantPublicColumns := map[string]string{
		"ID": "mpg-123", "Name": "test-cluster", "Organization": "test-org",
		"Region": "ord", "Status": "ready",
		"Total Used Storage (GB)": "1.2", "Total Allocated Storage (GB)": "20.0",
		"Replicas": "1", "Direct IP": "10.0.0.1",
	}
	wantPublicExcludes := []string{"10.0.0.1:5432", ":5432"}

	tests := []struct {
		name            string
		publicCluster   flaps.ManagedPostgresCluster
		publicErr       error
		legacyResponse  mpgv2.GetClusterResponse
		legacyErr       error
		wantColumns     map[string]string
		wantExcludes    []string
		wantErr         string
		fallback        bool // true when 404 triggers legacy fallback
		wantPublicErr   string
		wantLegacyCalls int
	}{
		{"public success with full 8-column mapping", samplePublicCluster(), nil, mpgv2.GetClusterResponse{}, nil, wantPublicColumns, wantPublicExcludes, "", false, "", 0},
		{"empty public host renders blank", func() flaps.ManagedPostgresCluster {
			c := samplePublicCluster()
			c.Endpoints.Primary.Direct.Host = ""

			return c
		}(), nil, mpgv2.GetClusterResponse{}, nil, map[string]string{"ID": "mpg-123", "Direct IP": ""}, []string{":5432", "5432"}, "", false, "", 0},
		{"classified 404 falls back to legacy with full mapping",
			flaps.ManagedPostgresCluster{}, fmt.Errorf("get Managed Postgres cluster: %w", &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: errors.New("not found")}),
			sampleLegacyCluster(), nil, map[string]string{"ID": "mpg-123", "Name": "test-cluster", "Organization": "test-org", "Region": "ord", "Status": "ready", "Total Used Storage (GB)": "1.2", "Total Allocated Storage (GB)": "20.0", "Replicas": "1", "Direct IP": "10.0.0.1"}, wantPublicExcludes, "", true, "", 1},
		{"404 with legacy failure preserves error",
			flaps.ManagedPostgresCluster{}, flaps.ErrFlapsNotFound,
			mpgv2.GetClusterResponse{}, errors.New("legacy denied"),
			nil, nil, "failed retrieving details for cluster mpg-123: legacy denied", false, "", 1},
		{"typed 500 is authoritative, no fallback",
			flaps.ManagedPostgresCluster{}, &flaps.FlapsError{ResponseStatusCode: 500, OriginalError: errors.New("oops")},
			mpgv2.GetClusterResponse{}, nil, nil, nil, "failed retrieving details for cluster mpg-123", false, "oops", 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := statusTestContext(t, false)
			publicCalls, legacyCalls := 0, 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
					publicCalls++
					require.Equal(t, "mpg-123", id)

					return test.publicCluster, test.publicErr
				},
			})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
					legacyCalls++
					require.Equal(t, "mpg-123", id)

					return test.legacyResponse, test.legacyErr
				},
			})

			err := RunStatus(ctx, "mpg-123")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Equal(t, 1, publicCalls)
				if test.legacyErr != nil {
					require.Equal(t, 1, legacyCalls)
				}
				if test.wantPublicErr != "" {
					require.ErrorContains(t, err, test.wantPublicErr)
				}
				require.Equal(t, test.wantLegacyCalls, legacyCalls)

				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, publicCalls)
			require.Equal(t, map[bool]int{true: 1, false: 0}[test.fallback], legacyCalls)

			out := stdout.String()
			for col, val := range test.wantColumns {
				require.Regexp(t, columnLineRegex(col, val), out, "column %q must render %q", col, val)
			}
			for _, miss := range test.wantExcludes {
				require.NotContains(t, out, miss)
			}
		})
	}
}

func TestRunStatusJSON(t *testing.T) {
	legacyResp := sampleLegacyCluster()
	wantCredentials := func(t *testing.T, got map[string]any) {
		data := got["data"].(map[string]any)
		require.Equal(t, "mpg-123", data["id"])
		require.Equal(t, "test-cluster", data["name"])
		require.Equal(t, "ord", data["region"])
		require.Equal(t, float64(10), data["disk"])
		require.Equal(t, float64(1), data["replicas"])
		ipAssignments := data["ip_assignments"].(map[string]any)
		require.Equal(t, "10.0.0.1", ipAssignments["direct"])
		creds := got["credentials"].(map[string]any)
		require.Equal(t, "app", creds["user"])
		require.Equal(t, "secret", creds["password"])
		require.Equal(t, "postgres://app:secret@10.0.0.1:5432/app", creds["pgbouncer_uri"])
	}

	tests := []struct {
		name       string
		legacyResp mpgv2.GetClusterResponse
		legacyErr  error
		wantErr    string
		wantCreds  bool
	}{
		{"json path preserves legacy envelope and credentials", legacyResp, nil, "", true},
		{"json path propagates legacy error without public fallback", mpgv2.GetClusterResponse{}, errors.New("legacy boom"), "failed retrieving details for cluster mpg-123: legacy boom", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := statusTestContext(t, true)
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					t.Fatal("json path must not touch the public API")

					return flaps.ManagedPostgresCluster{}, nil
				},
			})
			legacyCalls := 0
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
				GetClusterByIdFunc: func(_ context.Context, id string) (mpgv2.GetClusterResponse, error) {
					legacyCalls++
					require.Equal(t, "mpg-123", id)

					return test.legacyResp, test.legacyErr
				},
			})

			err := RunStatus(ctx, "mpg-123")
			require.Equal(t, 1, legacyCalls)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Empty(t, stdout.String(), "no JSON output on error")

				return
			}
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			if test.wantCreds {
				wantCredentials(t, got)
			}
		})
	}
}

func TestGbString(t *testing.T) {
	const gib = int64(1 << 30)
	i64 := func(v int64) *int64 { return &v }

	cases := []struct {
		name  string
		bytes *int64
		want  string
	}{
		{"nil reads N/A, not blank or 0", nil, "N/A"},
		{"whole GiB keeps one decimal", i64(20 * gib), "20.0"},
		{"exact fraction", i64(1288490188), "1.2"},
		{"divides by 1024^3, not 10^9", i64(10_000_000_000), "9.3"},
		{"rounds down below .x5", i64(65970697666), "61.4"},
		{"rounds up above .x5", i64(65992172503), "61.5"},
		{"small values do not collapse to 0", i64(64424509), "0.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, gbString(tc.bytes))
		})
	}
}
