package launch

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
)

func serveImage(t *testing.T, repo string, exposed ...string) string {
	t.Helper()

	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)

	img, err := random.Image(64, 1)
	require.NoError(t, err)
	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	cfg = cfg.DeepCopy()
	cfg.Config.ExposedPorts = map[string]struct{}{}
	for _, p := range exposed {
		cfg.Config.ExposedPorts[p] = struct{}{}
	}
	img, err = mutate.ConfigFile(img, cfg)
	require.NoError(t, err)

	ref := strings.TrimPrefix(srv.URL, "http://") + "/" + repo + ":latest"
	r, err := name.ParseReference(ref)
	require.NoError(t, err)
	require.NoError(t, remote.Write(r, img))

	return ref
}

func TestBuildManifestDatabaseImageHasNoHTTPService(t *testing.T) {
	for _, tc := range []struct {
		repo    string
		exposed []string
	}{
		{repo: "redis", exposed: []string{"6379/tcp"}},
		{repo: "postgres", exposed: []string{"5432/tcp"}},
		{repo: "mysql", exposed: []string{"3306/tcp", "33060/tcp"}},
		{repo: "mongo", exposed: []string{"27017/tcp"}},
		{repo: "memcached", exposed: []string{"11211/tcp"}},
		{repo: "apache/kafka", exposed: []string{"9092/tcp"}},
		{repo: "mqtt-broker", exposed: []string{"1883/tcp"}},
		{repo: "mssql/server", exposed: []string{"1433/tcp"}},
	} {
		t.Run(tc.repo, func(t *testing.T) {
			ref := serveImage(t, tc.repo, tc.exposed...)

			res, err := newManifestEnv().buildManifestFrom(t, "my-db", nil, "--image", ref)
			require.NoError(t, err)

			assert.Zero(t, res.manifest.Plan.HttpServicePort, "no HTTP service")
		})
	}
}

func TestBuildManifestImageUsesItsExposedPort(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exposed  []string
		wantPort int
	}{
		{name: "one port", exposed: []string{"80/tcp"}, wantPort: 80},
		{name: "one port without a protocol", exposed: []string{"3000"}, wantPort: 3000},
		{name: "no ports", wantPort: 8080},
		{name: "several ports", exposed: []string{"80/tcp", "443/tcp"}, wantPort: 8080},
		{name: "only UDP", exposed: []string{"53/udp"}, wantPort: 8080},
		{name: "port 0", exposed: []string{"0/tcp"}, wantPort: 8080},
		{name: "port above 65535", exposed: []string{"70000/tcp"}, wantPort: 8080},
		{name: "negative port", exposed: []string{"-1/tcp"}, wantPort: 8080},
		{name: "port 0 beside a valid port", exposed: []string{"0/tcp", "80/tcp"}, wantPort: 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := serveImage(t, "web", tc.exposed...)

			res, err := newManifestEnv().buildManifestFrom(t, "my-app", nil, "--image", ref)
			require.NoError(t, err)

			assert.Equal(t, tc.wantPort, res.manifest.Plan.HttpServicePort)
			assert.NotContains(t, res.errOut, "couldn't read the exposed ports")
		})
	}
}

func TestBuildManifestImageKeepsTodaysPlan(t *testing.T) {
	t.Run("image that can't be inspected", func(t *testing.T) {
		ref := serveImage(t, "other") + "-missing"

		res, err := newManifestEnv().buildManifestFrom(t, "my-app", nil, "--image", ref)
		require.NoError(t, err)

		assert.Equal(t, 8080, res.manifest.Plan.HttpServicePort)
		assert.Contains(t, res.errOut, "Warning: couldn't read the exposed ports of "+ref+" (Unknown manifest); using internal_port 8080. Set --internal-port to choose the port.\n")
	})

	t.Run("--internal-port asks for a service", func(t *testing.T) {
		ref := serveImage(t, "redis", "6379/tcp")

		res, err := newManifestEnv().buildManifestFrom(t, "my-db", nil, "--image", ref, "--internal-port", "6379")
		require.NoError(t, err)

		assert.NotZero(t, res.manifest.Plan.HttpServicePort)
	})

	t.Run("copied fly.toml", func(t *testing.T) {
		ref := serveImage(t, "redis", "6379/tcp")
		env := newManifestEnv()
		env.config = appconfig.NewConfig()
		env.config.HTTPService = &appconfig.HTTPService{InternalPort: 3000}

		res, err := env.buildManifestFrom(t, "my-db", nil, "--image", ref, "--copy-config")
		require.NoError(t, err)

		assert.Equal(t, 3000, res.manifest.Plan.HttpServicePort)
	})
}

func TestLaunchDatabaseImage(t *testing.T) {
	ref := serveImage(t, "redis", "6379/tcp")

	res, out, err := newManifestEnv().launchFrom(t, nil, "--image", ref, "--name", "my-db")
	require.NoError(t, err)

	cfg := loadFlyToml(t, res.dir)
	assert.Nil(t, cfg.HTTPService)
	assert.Empty(t, cfg.Services)
	assert.Contains(t, out, "my-db has no public HTTP service: "+ref+" exposes 6379, the Redis port.\n"+
		"From other apps in this org: my-db.internal:6379. From this computer: `fly proxy 6379 -a my-db`.\n"+
		"To add a public service, add it to fly.toml (https://docs.fly.io/reference/configuration#the-services-sections), then run `fly deploy`, `fly ips allocate-v6` and `fly ips allocate-v4 --shared`.\n")
	assert.NotContains(t, out, "flycast")
}

func TestLaunchDatabaseImageWithFlycast(t *testing.T) {
	ref := serveImage(t, "redis", "6379/tcp")

	_, out, err := newManifestEnv().launchFrom(t, nil, "--image", ref, "--name", "my-db", "--flycast")
	require.NoError(t, err)

	assert.Contains(t, out, "my-db.flycast has no address: Flycast needs a [[services]] entry for port 6379, then `fly deploy` and `fly ips allocate-v6 --private`.\n")
}

func TestLaunchWebImage(t *testing.T) {
	ref := serveImage(t, "nginx", "80/tcp")

	res, out, err := newManifestEnv().launchFrom(t, nil, "--image", ref, "--name", "my-web")
	require.NoError(t, err)

	cfg := loadFlyToml(t, res.dir)
	require.NotNil(t, cfg.HTTPService)
	assert.Equal(t, 80, cfg.HTTPService.InternalPort)
	assert.NotContains(t, out, "no public HTTP service")
}

func TestLaunchMongoImageHint(t *testing.T) {
	const hint = `launch with --command "mongod --bind_ip_all --ipv6"`

	for _, tc := range []struct {
		name     string
		exposed  string
		args     []string
		wantHint bool
	}{
		{name: "MongoDB", exposed: "27017/tcp", wantHint: true},
		{name: "MongoDB with its own command", exposed: "27017/tcp", args: []string{"--command", "mongod --bind_ip_all --ipv6"}},
		{name: "Redis", exposed: "6379/tcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := serveImage(t, "db", tc.exposed)

			_, out, err := newManifestEnv().launchFrom(t, nil, append([]string{"--image", ref, "--name", "my-db"}, tc.args...)...)
			require.NoError(t, err)

			assert.Contains(t, out, "my-db has no public HTTP service")
			if tc.wantHint {
				assert.Contains(t, out, hint)
			} else {
				assert.NotContains(t, out, hint)
			}
		})
	}
}

func TestBuildManifestImageLookupWarning(t *testing.T) {
	rateLimited := func(t *testing.T, rawURL string) error {
		req := httptest.NewRequest(http.MethodGet, rawURL, nil)
		body := `{"errors":[{"code":"TOOMANYREQUESTS","message":"You have reached your unauthenticated pull rate limit."}]}`
		err := transport.CheckError(&http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(body)), Request: req}, http.StatusOK)
		require.Error(t, err)
		return err
	}

	for _, tc := range []struct {
		name   string
		image  string
		err    func(t *testing.T) error
		reason string
	}{
		{
			name:  "Docker Hub rate limit",
			image: "redis",
			err: func(t *testing.T) error {
				return rateLimited(t, "https://index.docker.io/v2/library/redis/manifests/latest")
			},
			reason: "Docker Hub rate limit reached",
		},
		{
			name:   "another registry's rate limit",
			image:  "ghcr.io/acme/cache:1",
			err:    func(t *testing.T) error { return rateLimited(t, "https://ghcr.io/v2/acme/cache/manifests/1") },
			reason: "ghcr.io rate limit reached",
		},
		{
			name:  "timeout",
			image: "redis",
			err: func(t *testing.T) error {
				return &url.Error{Op: "Get", URL: "https://index.docker.io/v2/", Err: context.DeadlineExceeded}
			},
			reason: "timed out after 10s",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookupErr := tc.err(t)
			prev := imageExposedPorts
			imageExposedPorts = func(context.Context, string) ([]int, error) { return nil, lookupErr }
			t.Cleanup(func() { imageExposedPorts = prev })

			res, err := newManifestEnv().buildManifestFrom(t, "my-app", nil, "--image", tc.image)
			require.NoError(t, err)

			assert.Equal(t, 8080, res.manifest.Plan.HttpServicePort)
			assert.Contains(t, res.errOut, "Warning: couldn't read the exposed ports of "+tc.image+" ("+tc.reason+"); using internal_port 8080. Set --internal-port to choose the port.\n")
		})
	}
}

func TestImageExposedPortsMakesOneRequestWhenRateLimited(t *testing.T) {
	var manifestGets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/manifests/") {
			w.WriteHeader(http.StatusOK)
			return
		}
		manifestGets.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"errors":[{"code":"TOOMANYREQUESTS","message":"You have reached your unauthenticated pull rate limit."}]}`)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	image := host + "/redis:latest"

	_, err := imageExposedPorts(context.Background(), image)
	require.Error(t, err)

	assert.Equal(t, int32(1), manifestGets.Load())
	assert.Equal(t, host+" rate limit reached", imageLookupReason(image, err))
}
