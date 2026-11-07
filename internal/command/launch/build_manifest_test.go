package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// These tests run buildManifest, the planning half of fly launch, over a real
// source directory with the real scanners. The Fly APIs are fakes. They pin
// down what a plan looks like for each kind of app, so a change to a scanner
// or to the plan builder that changes what launch would do shows up here.

// manifestEnv is the fake platform a launch plan is built against.
type manifestEnv struct {
	// org is the only organization the user belongs to.
	org uiex.Organization
	// taken lists app names that already exist.
	taken map[string]bool
	// region is where placements puts the app when asked for "any" region.
	region string
	// noCapacity makes placements find no region with room for the app.
	noCapacity bool
	// config is a fly.toml already loaded from the source directory.
	config *appconfig.Config
	// mpgEnabled is what the Managed Postgres feature flag says.
	mpgEnabled bool

	// placements records each placements request.
	placements []flaps.GetPlacementsRequest
	// created records each app created.
	created []flaps.CreateAppRequest
}

func newManifestEnv() *manifestEnv {
	return &manifestEnv{
		org: uiex.Organization{
			Slug: "personal", RawSlug: "someone-123", Personal: true,
			Billable: true, BillingStatus: uiex.BillingStatusCurrent,
		},
		taken:  map[string]bool{},
		region: "ord",
	}
}

type manifestResult struct {
	// ctx is the context the plan was built in, for launching it.
	ctx      context.Context
	manifest *LaunchManifest
	cache    *planBuildCache
	dir      string
	out      string
	errOut   string
}

// buildManifestFrom writes files into a directory named dirName, then builds a
// launch plan for it, headless, as `fly launch --path <dir> args...` would.
// The working directory is the source directory, as it is for `fly launch`
// run in an app's directory.
func (e *manifestEnv) buildManifestFrom(t *testing.T, dirName string, files map[string]string, args ...string) (manifestResult, error) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), dirName)
	writeFiles(t, dir, files)
	t.Chdir(dir)

	return e.buildManifestIn(t, dir, args...)
}

func (e *manifestEnv) buildManifestIn(t *testing.T, dir string, args ...string) (manifestResult, error) {
	t.Helper()

	// GitHub Actions detection reads the real repository; keep it out.
	t.Setenv("OPT_OUT_GITHUB_ACTIONS", "1")
	unsetCIEnv(t)

	prev := managedPostgresEnabled
	managedPostgresEnabled = func() (bool, error) { return e.mpgEnabled, nil }
	t.Cleanup(func() { managedPostgresEnabled = prev })

	ios, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)

	flags := New().Flags()
	require.NoError(t, flags.Parse(append([]string{"--path", dir}, args...)))
	ctx = flagctx.NewContext(ctx, flags)
	if e.config != nil {
		ctx = appconfig.WithConfig(ctx, e.config)
	}

	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{e.org}, nil
		},
		GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
			org := e.org
			return &org, nil
		},
	})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
			return nil, errors.New("app not found")
		},
		AppNameAvailableFunc: func(_ context.Context, name string) (bool, error) {
			return !e.taken[name], nil
		},
		GetPlacementsFunc: func(_ context.Context, req *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error) {
			e.placements = append(e.placements, *req)
			if e.noCapacity {
				return nil, nil
			}
			region := req.Region
			if region == "any" {
				region = e.region
			}

			return []flaps.RegionPlacement{{Region: region, Count: int(req.Count)}}, nil
		},
		CreateAppFunc: func(_ context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
			e.created = append(e.created, req)

			return &flaps.App{Name: req.Name, Organization: flaps.AppOrganizationInfo{Slug: req.Org}}, nil
		},
		WaitForAppFunc: func(context.Context, string) error { return nil },
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{
				Nearest: e.region,
				Regions: []fly.Region{{Code: e.region, MPGAvailable: true}, {Code: "lhr"}},
			}, nil
		},
	})

	m, cache, err := buildManifest(ctx, nil, &recoverableErrorBuilder{canEnterUi: false})

	return manifestResult{ctx: ctx, manifest: m, cache: cache, dir: dir, out: out.String(), errOut: errOut.String()}, err
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, contents := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	}
}

// Source trees for the scanners that only read files. Rails, Phoenix,
// Laravel and the JavaScript frameworks run their toolchains while
// scanning, so they are covered in the scanner package instead.
var (
	goApp = map[string]string{
		"go.mod":  "module example.com/app\n\ngo 1.22\n",
		"go.sum":  "",
		"main.go": "package main\n\nfunc main() {}\n",
	}
	dockerfileApp = map[string]string{
		"Dockerfile": "FROM nginx:alpine\nEXPOSE 3000\n",
	}
	rustApp = map[string]string{
		"Cargo.toml": "[package]\nname = \"hello\"\nversion = \"0.1.0\"\n\n[dependencies]\naxum = \"0.7\"\n",
		"Cargo.lock": "",
	}
	denoApp = map[string]string{
		"deno.json": "{}\n",
		"main.ts":   "Deno.serve(() => new Response(\"hi\"))\n",
	}
	staticApp = map[string]string{
		"index.html": "<h1>hi</h1>\n",
	}
	djangoApp = map[string]string{
		"requirements.txt": "Django==5.0\npsycopg[binary]==3.1\ndj-database-url==2.1\nredis==5.0\nboto3==1.34\n",
		"manage.py":        "",
		"mysite/wsgi.py":   "",
	}
)

func TestBuildManifestFromSource(t *testing.T) {
	for _, tc := range []struct {
		name       string
		files      map[string]string
		wantFamily string
		wantPort   int
		wantDB     bool
		wantRedis  bool
		wantTigris bool
	}{
		{name: "Go", files: goApp, wantFamily: "Go", wantPort: 8080},
		{name: "Dockerfile uses its EXPOSE port", files: dockerfileApp, wantFamily: "Dockerfile", wantPort: 3000},
		{name: "Rust names the web framework", files: rustApp, wantFamily: "Axum", wantPort: 8080},
		{name: "Deno", files: denoApp, wantFamily: "Deno", wantPort: 8080},
		{name: "static site", files: staticApp, wantFamily: "Static", wantPort: 8080},
		{
			name: "Django asks for what its dependencies use", files: djangoApp,
			wantFamily: "Django", wantPort: 8000, wantDB: true, wantRedis: true, wantTigris: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newManifestEnv()
			res, err := env.buildManifestFrom(t, "my-app", tc.files)
			require.NoError(t, err)

			p := res.manifest.Plan
			assert.Equal(t, tc.wantFamily, p.ScannerFamily)
			assert.Equal(t, tc.wantPort, p.HttpServicePort)
			assert.True(t, p.HttpServicePortSetByScanner)

			assert.Equal(t, "my-app", p.AppName)
			assert.Equal(t, "my-app", res.manifest.Config.AppName)
			assert.Equal(t, appNameDerivedFromDir, res.manifest.PlanSource.appNameSource)
			assert.Equal(t, "personal", p.OrgSlug)
			assert.Equal(t, "ord", p.RegionCode)

			// Every app starts on one shared CPU with 1GB.
			assert.Equal(t, "shared", p.CPUKind)
			assert.Equal(t, 1, p.CPUs)
			assert.Equal(t, 1024, p.MemoryMB)
			// Two machines by default, so one can stop without downtime.
			assert.True(t, p.HighAvailability)

			assert.Equal(t, tc.wantDB, p.Postgres.FlyPostgres != nil, "postgres planned")
			assert.Nil(t, p.Postgres.ManagedPostgres)
			assert.Equal(t, tc.wantRedis, p.Redis.UpstashRedis != nil, "redis planned")
			assert.Equal(t, tc.wantTigris, p.ObjectStorage.TigrisObjectStorage != nil, "tigris planned")

			// The cache hands the scan to stateFromManifest so it isn't redone.
			require.NotNil(t, res.cache)
			require.NotNil(t, res.cache.sourceInfo)
			assert.Equal(t, tc.wantFamily, res.cache.sourceInfo.Family)
			assert.True(t, res.cache.billingChecked)

			require.Len(t, env.placements, 1)
			assert.Equal(t, "any", env.placements[0].Region)
			assert.Equal(t, uint64(2), env.placements[0].Count, "room for both machines")
			assert.Equal(t, "personal", env.placements[0].Org)
		})
	}
}

func TestBuildManifestOptOutFlags(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantDB     bool
		wantRedis  bool
		wantTigris bool
	}{
		{name: "none", wantDB: true, wantRedis: true, wantTigris: true},
		{name: "--no-db", args: []string{"--no-db"}, wantRedis: true, wantTigris: true},
		{name: "--no-redis", args: []string{"--no-redis"}, wantDB: true, wantTigris: true},
		{name: "--no-object-storage", args: []string{"--no-object-storage"}, wantDB: true, wantRedis: true},
		{name: "all three", args: []string{"--no-db", "--no-redis", "--no-object-storage"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := newManifestEnv().buildManifestFrom(t, "my-app", djangoApp, tc.args...)
			require.NoError(t, err)

			p := res.manifest.Plan
			assert.Equal(t, tc.wantDB, p.Postgres.Provider() != nil, "postgres planned")
			assert.Equal(t, tc.wantRedis, p.Redis.Provider() != nil, "redis planned")
			assert.Equal(t, tc.wantTigris, p.ObjectStorage.Provider() != nil, "tigris planned")

			wantSource := "not requested"
			if tc.wantDB {
				wantSource = "determined from app source"
			}
			assert.Equal(t, wantSource, res.manifest.PlanSource.postgresSource)
		})
	}
}

func TestBuildManifestDbFlag(t *testing.T) {
	t.Run("forces postgres for an app that doesn't ask for it", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--db=upg")
		require.NoError(t, err)

		assert.NotNil(t, res.manifest.Plan.Postgres.FlyPostgres)
		assert.Equal(t, "forced by --db flag", res.manifest.PlanSource.postgresSource)
	})
}

// The Managed Postgres flag only changes interactive launches; headless ones
// keep Unmanaged Postgres so scripts don't start paying for a cluster.
func TestBuildManifestManagedPostgresFlag(t *testing.T) {
	t.Run("headless with the flag on", func(t *testing.T) {
		env := newManifestEnv()
		env.mpgEnabled = true

		res, err := env.buildManifestFrom(t, "my-app", djangoApp)
		require.NoError(t, err)

		assert.NotNil(t, res.manifest.Plan.Postgres.FlyPostgres)
		assert.Nil(t, res.manifest.Plan.Postgres.ManagedPostgres)
		assert.Contains(t, res.errOut, "non-interactive launch defaults to Unmanaged Postgres")
	})

	t.Run("headless in a region without it", func(t *testing.T) {
		env := newManifestEnv()
		env.mpgEnabled = true

		res, err := env.buildManifestFrom(t, "my-app", djangoApp, "--region", "lhr")
		require.NoError(t, err)

		assert.NotNil(t, res.manifest.Plan.Postgres.FlyPostgres)
		assert.Contains(t, res.errOut, "Managed Postgres isn't yet available in region lhr")
	})

	t.Run("--db=mpg", func(t *testing.T) {
		env := newManifestEnv()
		env.mpgEnabled = true

		res, err := env.buildManifestFrom(t, "my-app", djangoApp, "--db=mpg")
		require.NoError(t, err)

		require.NotNil(t, res.manifest.Plan.Postgres.ManagedPostgres)
		assert.Nil(t, res.manifest.Plan.Postgres.FlyPostgres)
		assert.Equal(t, "ord", res.manifest.Plan.Postgres.ManagedPostgres.Region)
		assert.Equal(t, "ord", res.manifest.Plan.RegionCode)
	})
}

func TestBuildManifestAppName(t *testing.T) {
	t.Run("--name", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--name", "chosen")
		require.NoError(t, err)

		assert.Equal(t, "chosen", res.manifest.Plan.AppName)
		assert.Equal(t, "specified on the command line", res.manifest.PlanSource.appNameSource)
	})

	t.Run("directory name is sanitized", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestFrom(t, "My_Cool App", goApp)
		require.NoError(t, err)

		assert.Equal(t, "my-cool-app", res.manifest.Plan.AppName)
	})

	t.Run("a taken directory name gets a free suffix", func(t *testing.T) {
		env := newManifestEnv()
		env.taken["my-app"] = true

		res, err := env.buildManifestFrom(t, "my-app", goApp)
		require.NoError(t, err)

		name := res.manifest.Plan.AppName
		assert.Regexp(t, `^my-app-[a-z]+-[a-z]+-\d+$`, name)
		assert.Equal(t, "generated", res.manifest.PlanSource.appNameSource)
		assert.Equal(t, name, res.manifest.Config.AppName)
	})

	t.Run("a taken --name gets a free suffix", func(t *testing.T) {
		env := newManifestEnv()
		env.taken["chosen"] = true

		res, err := env.buildManifestFrom(t, "my-app", goApp, "--name", "chosen")
		require.NoError(t, err)

		assert.Regexp(t, `^chosen-[a-z]+-[a-z]+-\d+$`, res.manifest.Plan.AppName)
	})

	t.Run("--generate-name ignores the directory", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--generate-name")
		require.NoError(t, err)

		assert.Regexp(t, `^[a-z]+-[a-z]+-\d+$`, res.manifest.Plan.AppName)
		assert.NotContains(t, res.manifest.Plan.AppName, "my-app")
	})

	t.Run("--force-name keeps a taken name", func(t *testing.T) {
		env := newManifestEnv()
		env.taken["chosen"] = true

		res, err := env.buildManifestFrom(t, "my-app", goApp, "--name", "chosen", "--force-name")
		require.NoError(t, err)

		assert.Equal(t, "chosen", res.manifest.Plan.AppName)
	})

	t.Run("--force-name needs --name", func(t *testing.T) {
		_, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--force-name")
		assert.ErrorContains(t, err, "app name required when using --force-name")
	})

	t.Run("an invalid --name fails headless", func(t *testing.T) {
		_, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--name", "Not_Valid")
		assert.ErrorContains(t, err, "lowercase letters, numbers, and dashes")
	})
}

func TestBuildManifestRegion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		flyToml    string
		wantAsked  string
		wantSource string
		wantRegion string
	}{
		{name: "nearest", wantAsked: "any", wantRegion: "ord", wantSource: "this is the fastest region for you"},
		{name: "--region", args: []string{"--region", "lhr"}, wantAsked: "lhr", wantRegion: "lhr", wantSource: "specified on the command line"},
		{name: "retired --region is replaced", args: []string{"--region", "atl"}, wantAsked: "dfw", wantRegion: "dfw", wantSource: "specified on the command line"},
		{
			name:      "fly.toml primary_region",
			args:      []string{"--copy-config"},
			flyToml:   "app = 'from-toml'\nprimary_region = 'syd'\n",
			wantAsked: "syd", wantRegion: "syd", wantSource: "from your fly.toml",
		},
		{
			name:      "--region beats fly.toml",
			args:      []string{"--copy-config", "--region", "lhr"},
			flyToml:   "app = 'from-toml'\nprimary_region = 'syd'\n",
			wantAsked: "lhr", wantRegion: "lhr", wantSource: "specified on the command line",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newManifestEnv()
			files := dockerfileApp
			var err error
			var res manifestResult
			if tc.flyToml != "" {
				res, err = env.buildManifestWithFlyToml(t, files, tc.flyToml, tc.args...)
			} else {
				res, err = env.buildManifestFrom(t, "my-app", files, tc.args...)
			}
			require.NoError(t, err)

			require.Len(t, env.placements, 1)
			assert.Equal(t, tc.wantAsked, env.placements[0].Region)
			assert.Equal(t, tc.wantRegion, res.manifest.Plan.RegionCode)
			assert.Equal(t, tc.wantSource, res.manifest.PlanSource.regionSource)
		})
	}

	t.Run("no capacity fails headless", func(t *testing.T) {
		env := newManifestEnv()
		env.noCapacity = true

		res, err := env.buildManifestFrom(t, "my-app", dockerfileApp)
		assert.ErrorContains(t, err, "no regions with capacity")
		// The partial plan carries what metrics need.
		require.NotNil(t, res.manifest)
		assert.Equal(t, "my-app", res.manifest.Plan.AppName)
		assert.Equal(t, "personal", res.manifest.Plan.OrgSlug)
		assert.Equal(t, "Dockerfile", res.manifest.Plan.ScannerFamily)
	})
}

// buildManifestWithFlyToml loads flyToml the way fly launch does when one is
// present, then builds the plan.
func (e *manifestEnv) buildManifestWithFlyToml(t *testing.T, files map[string]string, flyToml string, args ...string) (manifestResult, error) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "my-app")
	files = maps.Clone(files)
	files["fly.toml"] = flyToml
	writeFiles(t, dir, files)
	t.Chdir(dir)

	cfg, err := appconfig.LoadConfig(filepath.Join(dir, "fly.toml"))
	require.NoError(t, err)
	e.config = cfg

	return e.buildManifestIn(t, dir, args...)
}

func TestBuildManifestCompute(t *testing.T) {
	t.Run("--vm-memory", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--vm-memory", "2048")
		require.NoError(t, err)

		assert.Equal(t, 2048, res.manifest.Plan.MemoryMB)
		assert.Equal(t, "specified on the command line", res.manifest.PlanSource.computeSource)
	})

	t.Run("--vm-size", func(t *testing.T) {
		env := newManifestEnv()
		res, err := env.buildManifestFrom(t, "my-app", goApp, "--vm-size", "performance-2x")
		require.NoError(t, err)

		assert.Equal(t, "performance", res.manifest.Plan.CPUKind)
		assert.Equal(t, 2, res.manifest.Plan.CPUs)
		// Placements are asked for the machine that will be launched.
		require.Len(t, env.placements, 1)
		assert.Equal(t, "performance", env.placements[0].ComputeRequirements.CPUKind)
	})

	t.Run("an unknown --vm-size fails headless", func(t *testing.T) {
		_, err := newManifestEnv().buildManifestFrom(t, "my-app", goApp, "--vm-size", "enormous")
		assert.ErrorContains(t, err, "invalid machine preset requested, 'enormous'")
	})
}

func TestBuildManifestHighAvailability(t *testing.T) {
	t.Run("--ha=false", func(t *testing.T) {
		env := newManifestEnv()
		res, err := env.buildManifestFrom(t, "my-app", goApp, "--ha=false")
		require.NoError(t, err)

		assert.False(t, res.manifest.Plan.HighAvailability)
		require.Len(t, env.placements, 1)
		assert.Equal(t, uint64(1), env.placements[0].Count)
	})

	t.Run("org without a payment method", func(t *testing.T) {
		env := newManifestEnv()
		env.org.Billable = false
		env.org.BillingStatus = uiex.BillingStatusTrialActive

		res, err := env.buildManifestFrom(t, "my-app", goApp)
		require.NoError(t, err)

		// The plan keeps asking for HA; Launch turns it off. The warning is
		// printed now, and only once.
		assert.Contains(t, res.errOut, "turning off high availability")
		assert.True(t, res.cache.warnedNoCcHa)
	})
}

func TestBuildManifestCopyConfig(t *testing.T) {
	flyToml := `app = 'from-toml'
primary_region = 'syd'

[http_service]
  internal_port = 4000
`

	t.Run("keeps the app name and port", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestWithFlyToml(t, map[string]string{"Dockerfile": "FROM nginx\n"}, flyToml, "--copy-config")
		require.NoError(t, err)

		assert.Equal(t, "from-toml", res.manifest.Plan.AppName)
		assert.Equal(t, "from your fly.toml", res.manifest.PlanSource.appNameSource)
		assert.Equal(t, 4000, res.manifest.Plan.HttpServicePort)
	})

	t.Run("--yes without --copy-config refuses to replace it", func(t *testing.T) {
		_, err := newManifestEnv().buildManifestWithFlyToml(t, map[string]string{"Dockerfile": "FROM nginx\n"}, flyToml, "--yes")
		assert.ErrorContains(t, err, "pass --copy-config")
	})

	t.Run("headless without --copy-config asks for it", func(t *testing.T) {
		_, err := newManifestEnv().buildManifestWithFlyToml(t, map[string]string{"Dockerfile": "FROM nginx\n"}, flyToml)
		assert.ErrorContains(t, err, "--copy-config must be specified")
	})

	t.Run("--copy-config=false rescans the source", func(t *testing.T) {
		res, err := newManifestEnv().buildManifestWithFlyToml(t, map[string]string{"Dockerfile": "FROM nginx\nEXPOSE 3000\n"}, flyToml, "--copy-config=false")
		require.NoError(t, err)

		assert.Equal(t, "my-app", res.manifest.Plan.AppName)
		assert.Equal(t, 3000, res.manifest.Plan.HttpServicePort)
	})
}

func TestBuildManifestNothingToBuild(t *testing.T) {
	_, err := newManifestEnv().buildManifestFrom(t, "my-app", map[string]string{"README.md": "hi\n"})
	assert.ErrorContains(t, err, "found nothing to build")
}

// fly launch --manifest prints the plan and --from-manifest launches it. The
// manifest has to survive that round trip, and launching it rescans the
// source, which has to still be the same kind of app.
func TestManifestRoundTrip(t *testing.T) {
	env := newManifestEnv()
	res, err := env.buildManifestFrom(t, "my-app", djangoApp)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(res.manifest))

	var loaded LaunchManifest
	require.NoError(t, json.Unmarshal(buf.Bytes(), &loaded))
	assert.Equal(t, res.manifest.Plan, loaded.Plan)
	assert.Equal(t, res.manifest.Config.AppName, loaded.Config.AppName)
	// PlanSource's fields are unexported, so it doesn't survive JSON; run
	// fills it in after loading a manifest.

	newCtx := func(t *testing.T, dir string) context.Context {
		ios, _, _, _ := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)
		flags := New().Flags()
		require.NoError(t, flags.Parse([]string{"--path", dir, "--no-deploy"}))
		ctx = flagctx.NewContext(ctx, flags)
		ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{})

		return ctx
	}

	t.Run("same source", func(t *testing.T) {
		state, err := stateFromManifest(newCtx(t, res.dir), loaded, nil, &recoverableErrorBuilder{})
		require.NoError(t, err)

		assert.Equal(t, "Django", state.sourceInfo.Family)
		assert.Equal(t, res.dir, state.workingDir)
		assert.Equal(t, filepath.Join(res.dir, "fly.toml"), state.configPath)
	})

	t.Run("source changed to another kind of app", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-app")
		writeFiles(t, dir, goApp)
		t.Chdir(dir)

		_, err := stateFromManifest(newCtx(t, dir), loaded, nil, &recoverableErrorBuilder{})
		assert.ErrorContains(t, err, "launch manifest was created for a Django app, but this is a Go app")
	})
}
