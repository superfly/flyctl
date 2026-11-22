package launch

import (
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := serveImage(t, "web", tc.exposed...)

			res, err := newManifestEnv().buildManifestFrom(t, "my-app", nil, "--image", ref)
			require.NoError(t, err)

			assert.Equal(t, tc.wantPort, res.manifest.Plan.HttpServicePort)
		})
	}
}

func TestBuildManifestImageKeepsTodaysPlan(t *testing.T) {
	t.Run("image that can't be inspected", func(t *testing.T) {
		ref := serveImage(t, "other") + "-missing"

		res, err := newManifestEnv().buildManifestFrom(t, "my-app", nil, "--image", ref)
		require.NoError(t, err)

		assert.Equal(t, 8080, res.manifest.Plan.HttpServicePort)
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
	assert.Contains(t, out, "my-db has no public HTTP service: "+ref+" exposes 6379, the Redis port.")
	assert.Contains(t, out, "Reach it at my-db.internal:6379, or from here with `fly proxy 6379 -a my-db`.")
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
