package cmdv2

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// createTestContext builds a context carrying a --region flag and an
// iostream suitable for the create command's expectations. If region is
// the empty string, --region is left unset (so the command takes the
// interactive-prompt branch instead).
func createTestContext(t *testing.T, region string) (context.Context, *bytes.Buffer) {
	t.Helper()

	io, _, stdout, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("region", "", "")
	if region != "" {
		require.NoError(t, flags.Set("region", region))
	}

	return flagctx.NewContext(ctx, flags), stdout
}

func sampleParams() *CreateClusterParams {
	return &CreateClusterParams{
		Name:           "my-cluster",
		OrgSlug:        "my-org",
		Plan:           "basic",
		StorageInGb:    10,
		PGMajorVersion: 16,
		PostGISEnabled: true,
	}
}

func samplePlan() *CreatePlanDisplay {
	return &CreatePlanDisplay{
		Name:       "Basic",
		CPU:        "1",
		Memory:     "256MB",
		PricePerMo: 5,
	}
}

// sampleRegions returns the region list returned by the mocked GetRegions
// call. Codes here drive the explicit --region validation, the empty-list
// short-circuit, and the interactive-prompt option list.
func sampleRegions() []fly.Region {
	return []fly.Region{
		{Code: "iad", Name: "Ashburn, Virginia (US)", MPGAvailable: true},
		{Code: "lax", Name: "Los Angeles, California (US)", MPGAvailable: true},
	}
}

// poolerEndpoints builds the inline-struct Endpoints value used by the
// credentials/URI tests. Tests that exercise the credentials branch set the
// cluster's Endpoints.Primary.Pooler so the expected URI is
// "postgres://fly-user:<password>@pooler.example.test:6432/fly-db".
func poolerEndpoints() flaps.ManagedPostgresEndpoints {
	return flaps.ManagedPostgresEndpoints{
		Primary: struct {
			Direct flaps.ManagedPostgresEndpoint `json:"direct"`
			Pooler flaps.ManagedPostgresEndpoint `json:"pooler"`
		}{
			Pooler: flaps.ManagedPostgresEndpoint{Host: "pooler.example.test", Port: 6432},
		},
	}
}

// assertNoLegacyCreate wires the MpgV2Client mock such that any call to the
// private CreateCluster fails the test. The no-create-fallback invariant
// (invariant #1 in the plan) requires that RunCreate never touch the
// private client for cluster creation.
func assertNoLegacyCreate(t *testing.T, ctx context.Context) context.Context {
	t.Helper()

	return mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		CreateClusterFunc: func(context.Context, mpgv2.CreateClusterInput) (mpgv2.CreateClusterResponse, error) {
			t.Fatal("mpgv2.CreateCluster must not be called by the public-API create path")

			return mpgv2.CreateClusterResponse{}, nil
		},
	})
}

// TestRunCreate_RegionDiscovery covers the regionsv2 -> mpgutil.AvailableRegions
// swap: explicit --region validation, empty-list short-circuit, and the
// invalid-region error's codes content.
func TestRunCreate_RegionDiscovery(t *testing.T) {
	tests := []struct {
		name          string
		regionFlag    string
		regions       []fly.Region
		regionsErr    error
		wantErr       string
		wantCreateHit bool
	}{
		{
			name:          "explicit valid region is selected and proceeds to create",
			regionFlag:    "lax",
			regions:       sampleRegions(),
			wantCreateHit: true,
		},
		{
			name:       "explicit invalid region returns error with available codes from the same call",
			regionFlag: "sfo",
			regions:    sampleRegions(),
			wantErr:    "region sfo is not available for Managed Postgres. Available regions: [iad lax]",
		},
		{
			name:    "empty region list returns legacy error unchanged",
			regions: nil,
			wantErr: "no valid regions found for Managed Postgres",
		},
		{
			name:       "regions retrieval failure is propagated",
			regionsErr: errors.New("regions boom"),
			wantErr:    "regions boom",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := createTestContext(t, test.regionFlag)
			createCalled := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
					if test.regionsErr != nil {
						return nil, test.regionsErr
					}

					return &flaps.RegionData{Regions: test.regions}, nil
				},
				CreateManagedPostgresClusterFunc: func(_ context.Context, req flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					createCalled = true
					require.Equal(t, "basic", req.Plan)
					require.Equal(t, "my-org", req.OrgSlug)
					require.Equal(t, "16", req.PGMajorVersion)
					require.Equal(t, 10, req.DiskSizeGB)
					require.Equal(t, "my-cluster", req.Name)
					require.Equal(t, "lax", req.Region)
					require.True(t, req.PostGISEnabled)

					return flaps.ManagedPostgresCluster{ID: "mpg-123", Region: req.Region, DiskSizeGB: req.DiskSizeGB, PostGISEnabled: req.PostGISEnabled}, nil
				},
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{ID: "mpg-123", Status: flaps.ManagedPostgresStatusReady, Endpoints: poolerEndpoints()}, nil
				},
				GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
					return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: "secret"}, nil
				},
			})
			ctx = assertNoLegacyCreate(t, ctx)

			err := RunCreate(ctx, "my-org", sampleParams(), samplePlan())
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantCreateHit, createCalled)
		})
	}
}

// TestRunCreate_InteractivePromptRegion verifies that when --region is not
// set, the region options are built from mpgutil.AvailableRegions (so the
// prompt path is wired to the same data source as the explicit path). In
// non-interactive test mode prompt.Select returns prompt.ErrNonInteractive;
// that is sufficient proof that the option list flowed through to survey.
func TestRunCreate_InteractivePromptRegion(t *testing.T) {
	ctx, _ := createTestContext(t, "" /* no --region */)
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: sampleRegions()}, nil
		},
	})
	ctx = assertNoLegacyCreate(t, ctx)

	err := RunCreate(ctx, "my-org", sampleParams(), samplePlan())
	require.Error(t, err)
	require.True(t, prompt.IsNonInteractive(err), "expected prompt.ErrNonInteractive, got: %v", err)
}

// TestRunCreate_PollLoop covers the two terminal-failure statuses documented
// for this endpoint ("failed" canonical, "error" backend-reported), the
// malformed-response guard, and a transient poll error.
func TestRunCreate_PollLoop(t *testing.T) {
	tests := []struct {
		name    string
		polls   []flaps.ManagedPostgresCluster
		pollErr error
		wantErr string
	}{
		{
			name: "ready on first poll proceeds to credentials",
			polls: []flaps.ManagedPostgresCluster{
				{ID: "mpg-123", Status: flaps.ManagedPostgresStatusReady, Endpoints: poolerEndpoints()},
			},
		},
		{
			name: "transient provisioning status keeps polling then succeeds",
			polls: []flaps.ManagedPostgresCluster{
				{ID: "mpg-123", Status: "creating"},
				{ID: "mpg-123", Status: flaps.ManagedPostgresStatusReady, Endpoints: poolerEndpoints()},
			},
		},
		{
			// Propagation-lag race between a cluster becoming ready and its
			// endpoint data being populated. The pooler endpoint is zero-valued
			// (no Host, no Port) but Status is still "ready". RunCreate must
			// surface this as an error from buildConnectionURI rather than
			// silently printing a broken URI such as
			// "postgres://fly-user:secret@:0/fly-db" as if the cluster had been
			// created successfully.
			name: "ready status with zero-value pooler endpoint returns an error (propagation-lag race)",
			polls: []flaps.ManagedPostgresCluster{
				{ID: "mpg-123", Status: flaps.ManagedPostgresStatusReady, Endpoints: flaps.ManagedPostgresEndpoints{}},
			},
			wantErr: "failed to build connection URI for cluster mpg-123: cluster ready but pooler endpoint not yet available",
		},
		{
			name: "failed status is terminal failure",
			polls: []flaps.ManagedPostgresCluster{
				{ID: "mpg-123", Status: flaps.ManagedPostgresStatusFailed},
			},
			wantErr: "cluster creation failed",
		},
		{
			name: "error status is also terminal failure (backend-reported spelling)",
			polls: []flaps.ManagedPostgresCluster{
				{ID: "mpg-123", Status: flaps.ManagedPostgresStatusError},
			},
			wantErr: "cluster creation failed",
		},
		{
			name:    "transient poll error is a hard error (no retry)",
			pollErr: errors.New("net down"),
			wantErr: "failed checking cluster status: net down",
		},
		{
			name: "missing cluster ID in poll response is a hard error",
			polls: []flaps.ManagedPostgresCluster{
				{ID: "", Status: "creating"},
			},
			wantErr: "invalid cluster response: no cluster ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := createTestContext(t, "iad")
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
					return &flaps.RegionData{Regions: sampleRegions()}, nil
				},
				CreateManagedPostgresClusterFunc: func(context.Context, flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{ID: "mpg-123", Region: "iad", DiskSizeGB: 10, PostGISEnabled: true}, nil
				},
				GetManagedPostgresClusterFunc: func(_ context.Context, id string) (flaps.ManagedPostgresCluster, error) {
					require.Equal(t, "mpg-123", id)
					if test.pollErr != nil {
						return flaps.ManagedPostgresCluster{}, test.pollErr
					}
					if len(test.polls) == 0 {
						t.Fatal("test setup error: no poll responses configured")

						return flaps.ManagedPostgresCluster{}, nil
					}
					p := test.polls[0]
					test.polls = test.polls[1:]

					return p, nil
				},
				GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
					return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: "secret"}, nil
				},
			})
			ctx = assertNoLegacyCreate(t, ctx)

			err := RunCreate(ctx, "my-org", sampleParams(), samplePlan())
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRunCreate_CreateError covers the no-fallback invariant: any error from
// CreateManagedPostgresCluster is returned to the caller without a private
// retry that could double-provision the cluster.
func TestRunCreate_CreateError(t *testing.T) {
	ctx, _ := createTestContext(t, "iad")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: sampleRegions()}, nil
		},
		CreateManagedPostgresClusterFunc: func(context.Context, flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			return flaps.ManagedPostgresCluster{}, errors.New("quota exceeded")
		},
	})
	ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{
		CreateClusterFunc: func(context.Context, mpgv2.CreateClusterInput) (mpgv2.CreateClusterResponse, error) {
			t.Fatal("create error must never trigger a legacy fallback that risks double-provisioning")

			return mpgv2.CreateClusterResponse{}, nil
		},
	})

	err := RunCreate(ctx, "my-org", sampleParams(), samplePlan())
	require.ErrorContains(t, err, "failed creating managed postgres cluster: quota exceeded")
}

// TestRunCreate_RequestMapping verifies the CreateClusterParams -> request
// field mapping, including the int -> string conversion of PG major version
// and the direct copy of storage GB.
func TestRunCreate_RequestMapping(t *testing.T) {
	tests := []struct {
		name       string
		params     *CreateClusterParams
		regionFlag string
		want       flaps.CreateManagedPostgresClusterRequest
	}{
		{
			name: "all fields populated",
			params: &CreateClusterParams{
				Name: "my-cluster", OrgSlug: "my-org", Plan: "performance",
				StorageInGb: 50, PGMajorVersion: 17, PostGISEnabled: true,
			},
			regionFlag: "iad",
			want: flaps.CreateManagedPostgresClusterRequest{
				Name: "my-cluster", OrgSlug: "my-org", Plan: "performance",
				Region: "iad", DiskSizeGB: 50, PGMajorVersion: "17", PostGISEnabled: true,
			},
		},
		{
			name: "postgis disabled and pg16",
			params: &CreateClusterParams{
				Name: "tiny", OrgSlug: "acme", Plan: "basic",
				StorageInGb: 1, PGMajorVersion: 16, PostGISEnabled: false,
			},
			regionFlag: "lax",
			want: flaps.CreateManagedPostgresClusterRequest{
				Name: "tiny", OrgSlug: "acme", Plan: "basic",
				Region: "lax", DiskSizeGB: 1, PGMajorVersion: "16", PostGISEnabled: false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := createTestContext(t, test.regionFlag)
			var got flaps.CreateManagedPostgresClusterRequest
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
					return &flaps.RegionData{Regions: sampleRegions()}, nil
				},
				CreateManagedPostgresClusterFunc: func(_ context.Context, req flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					got = req

					return flaps.ManagedPostgresCluster{ID: "mpg-123", Region: req.Region, DiskSizeGB: req.DiskSizeGB, PostGISEnabled: req.PostGISEnabled}, nil
				},
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{ID: "mpg-123", Status: flaps.ManagedPostgresStatusReady, Endpoints: poolerEndpoints()}, nil
				},
				GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
					return flaps.ManagedPostgresUserCredentials{Username: "fly-user", Password: "secret"}, nil
				},
			})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{})

			require.NoError(t, RunCreate(ctx, "my-org", test.params, samplePlan()))
			require.Equal(t, test.want, got)
		})
	}
}

// TestRunCreate_Credentials covers the post-ready branch: fetching the
// fly-user credentials, composing the connection URI from the pooler
// endpoint, and erroring when the credentials fetch fails (no retry).
func TestRunCreate_Credentials(t *testing.T) {
	tests := []struct {
		name           string
		password       string
		credsUsername  string
		credsErr       error
		wantInOutput   []string
		wantNotInOut   []string
		wantErr        string
		wantCredsCalls int
	}{
		{
			name:           "happy path prints pooler-based URI",
			password:       "secret",
			credsUsername:  "fly-user",
			wantInOutput:   []string{"postgres://fly-user:secret@pooler.example.test:6432/fly-db", "Connection string: "},
			wantCredsCalls: 1,
		},
		{
			name:          "password containing reserved URI characters is escaped",
			password:      "p@ss:wo/rd%",
			credsUsername: "fly-user",
			// url.UserPassword percent-escapes '@', ':', '/', '%' (and others)
			// when rendering the userinfo, so the raw password must NOT appear
			// inside the printed URI.
			wantInOutput: []string{
				"postgres://fly-user:" + url.QueryEscape("p@ss:wo/rd%") + "@pooler.example.test:6432/fly-db",
			},
			wantNotInOut:   []string{"fly-user:p@ss:wo/rd%@"},
			wantCredsCalls: 1,
		},
		{
			name:     "unexpected username in credentials response is ignored; URI keeps fixed fly-user",
			password: "secret",
			// The plan pins the URI username to the literal "fly-user" regardless
			// of any Username value the credentials endpoint echoes back; this
			// guards against a regression where the URI user drifts to the
			// payload-supplied value (e.g. "unexpected-user").
			credsUsername: "unexpected-user",
			wantInOutput: []string{
				"postgres://fly-user:secret@pooler.example.test:6432/fly-db",
			},
			wantNotInOut: []string{
				"postgres://unexpected-user:secret@",
			},
			wantCredsCalls: 1,
		},
		{
			name:          "empty username in credentials response is ignored; URI keeps fixed fly-user",
			password:      "secret",
			credsUsername: "",
			wantInOutput: []string{
				"postgres://fly-user:secret@pooler.example.test:6432/fly-db",
			},
			wantCredsCalls: 1,
		},
		{
			name:           "credentials fetch failure is a hard error with no retry",
			credsErr:       errors.New("creds 500"),
			wantErr:        "failed retrieving credentials for cluster mpg-123: creds 500",
			wantCredsCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stdout := createTestContext(t, "iad")
			credsCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
					return &flaps.RegionData{Regions: sampleRegions()}, nil
				},
				CreateManagedPostgresClusterFunc: func(context.Context, flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{ID: "mpg-123", Region: "iad", DiskSizeGB: 10, PostGISEnabled: true}, nil
				},
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					return flaps.ManagedPostgresCluster{ID: "mpg-123", Status: flaps.ManagedPostgresStatusReady, Endpoints: poolerEndpoints()}, nil
				},
				GetManagedPostgresUserCredentialsFunc: func(_ context.Context, id, username string) (flaps.ManagedPostgresUserCredentials, error) {
					credsCalls++
					require.Equal(t, "mpg-123", id)
					require.Equal(t, "fly-user", username)
					if test.credsErr != nil {
						return flaps.ManagedPostgresUserCredentials{}, test.credsErr
					}

					return flaps.ManagedPostgresUserCredentials{Username: test.credsUsername, Password: test.password}, nil
				},
			})
			ctx = mpgv2.NewContextWithClient(ctx, &mock.MpgV2Client{})

			err := RunCreate(ctx, "my-org", sampleParams(), samplePlan())
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Equal(t, test.wantCredsCalls, credsCalls)

				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantCredsCalls, credsCalls)

			out := stdout.String()
			for _, want := range test.wantInOutput {
				require.True(t, strings.Contains(out, want), "stdout must contain %q\nstdout: %s", want, out)
			}
			for _, miss := range test.wantNotInOut {
				require.False(t, strings.Contains(out, miss), "stdout must not contain unescaped %q\nstdout: %s", miss, out)
			}
		})
	}
}

// TestBuildConnectionURI is a focused unit test for the URL-construction
// helper. It pins down the exact escape behavior so a regression in
// url.UserPassword's output (or a switch away from it) is caught even if
// RunCreate's larger wiring changes.
func TestBuildConnectionURI(t *testing.T) {
	tests := []struct {
		name     string
		endpoint flaps.ManagedPostgresEndpoint
		user     string
		pass     string
		db       string
		want     string
		wantErr  string
	}{
		{
			name:     "plain credentials",
			endpoint: flaps.ManagedPostgresEndpoint{Host: "h.test", Port: 5432},
			user:     "u",
			pass:     "p",
			db:       "d",
			want:     "postgres://u:p@h.test:5432/d",
		},
		{
			name:     "reserved URI characters in password are escaped",
			endpoint: flaps.ManagedPostgresEndpoint{Host: "h.test", Port: 5432},
			user:     "u",
			pass:     "p@ss:wo/rd%",
			db:       "d",
			// url.UserPassword escapes '@', ':', '/', '%' — which would otherwise
			// corrupt the URI's userinfo, host, path, or percent-decoding.
			want: "postgres://u:" + url.QueryEscape("p@ss:wo/rd%") + "@h.test:5432/d",
		},
		{
			// Propagation-lag race: a cluster can report status "ready" before
			// its pooler endpoint data is populated. Without this guard,
			// buildConnectionURI would silently return a broken URI such as
			// "postgres://fly-user:secret@:0/fly-db" with a nil error and
			// RunCreate would print it as if the cluster had been created
			// successfully.
			name:     "zero-value pooler endpoint (empty host, zero port) returns error instead of broken URI",
			endpoint: flaps.ManagedPostgresEndpoint{},
			user:     "fly-user",
			pass:     "secret",
			db:       "fly-db",
			wantErr:  "cluster ready but pooler endpoint not yet available",
		},
		{
			name:     "missing host with non-zero port returns error",
			endpoint: flaps.ManagedPostgresEndpoint{Port: 6432},
			user:     "u",
			pass:     "p",
			db:       "d",
			wantErr:  "cluster ready but pooler endpoint not yet available",
		},
		{
			name:     "host with zero port returns error",
			endpoint: flaps.ManagedPostgresEndpoint{Host: "h.test"},
			user:     "u",
			pass:     "p",
			db:       "d",
			wantErr:  "cluster ready but pooler endpoint not yet available",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildConnectionURI(test.endpoint, test.user, test.pass, test.db)
			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				require.Empty(t, got, "buildConnectionURI must not return a URI when it returns an error")

				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
