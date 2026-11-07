package launch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/iostreams"
)

// launchFrom runs the whole of a headless fly launch over files, short of
// deploying: plan, launch state, then Launch, which creates the app and
// writes fly.toml and the files the scanner generates.
func (e *manifestEnv) launchFrom(t *testing.T, files map[string]string, args ...string) (manifestResult, string, error) {
	t.Helper()

	res, err := e.buildManifestFrom(t, "my-app", files, append([]string{"--no-deploy"}, args...)...)
	require.NoError(t, err)

	ios, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(res.ctx, ios)

	state, err := stateFromManifest(ctx, *res.manifest, res.cache, &recoverableErrorBuilder{})
	require.NoError(t, err)

	err = state.Launch(ctx)

	return res, out.String(), err
}

func loadFlyToml(t *testing.T, dir string) *appconfig.Config {
	t.Helper()

	cfg, err := appconfig.LoadConfig(filepath.Join(dir, "fly.toml"))
	require.NoError(t, err)

	return cfg
}

func TestLaunchGoApp(t *testing.T) {
	env := newManifestEnv()
	res, out, err := env.launchFrom(t, goApp)
	require.NoError(t, err)

	require.Len(t, env.created, 1)
	assert.Equal(t, "my-app", env.created[0].Name)
	assert.Equal(t, "personal", env.created[0].Org)
	assert.Contains(t, out, "Created app 'my-app' in organization 'personal'")
	assert.Contains(t, out, "Hostname: my-app.fly.dev")

	cfg := loadFlyToml(t, res.dir)
	assert.Equal(t, "my-app", cfg.AppName)
	assert.Equal(t, "ord", cfg.PrimaryRegion)
	require.NotNil(t, cfg.HTTPService)
	assert.Equal(t, 8080, cfg.HTTPService.InternalPort)
	assert.True(t, cfg.HTTPService.ForceHTTPS)
	assert.Equal(t, "8080", cfg.Env["PORT"])
	require.NotNil(t, cfg.Build)
	assert.Equal(t, "1.22", cfg.Build.Args["GO_VERSION"])
	require.Len(t, cfg.Compute, 1)
	assert.Equal(t, "1gb", cfg.Compute[0].Memory)

	// The Go scanner brings its own Dockerfile.
	dockerfile, err := os.ReadFile(filepath.Join(res.dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, string(dockerfile), "ARG GO_VERSION")
	// Without a .gitignore there's nothing to make a .dockerignore from.
	assert.NoFileExists(t, filepath.Join(res.dir, ".dockerignore"))

	// fly.toml is valid for fly deploy.
	require.NoError(t, cfg.SetMachinesPlatform())
	err, extra := cfg.Validate(res.ctx)
	assert.NoError(t, err, extra)
}

func TestLaunchFlagsReachFlyToml(t *testing.T) {
	env := newManifestEnv()
	res, _, err := env.launchFrom(t, dockerfileApp,
		"--name", "chosen",
		"--region", "lhr",
		"--vm-memory", "512",
		"--internal-port", "9000",
		"--command", "nginx -g 'daemon off;'",
		"--volume", "data:/data",
		"--env", "FOO=bar",
	)
	require.NoError(t, err)

	require.Len(t, env.created, 1)
	assert.Equal(t, "chosen", env.created[0].Name)

	cfg := loadFlyToml(t, res.dir)
	assert.Equal(t, "chosen", cfg.AppName)
	assert.Equal(t, "lhr", cfg.PrimaryRegion)
	require.NotNil(t, cfg.HTTPService)
	assert.Equal(t, 9000, cfg.HTTPService.InternalPort, "--internal-port beats EXPOSE")
	assert.Equal(t, "nginx -g 'daemon off;'", cfg.Processes["app"])
	require.Len(t, cfg.Mounts, 1)
	assert.Equal(t, "data", cfg.Mounts[0].Source)
	assert.Equal(t, "/data", cfg.Mounts[0].Destination)
	assert.Equal(t, "bar", cfg.Env["FOO"])
	require.Len(t, cfg.Compute, 1)
	assert.Equal(t, "512mb", cfg.Compute[0].Memory)

	// A Dockerfile app keeps the user's Dockerfile.
	dockerfile, err := os.ReadFile(filepath.Join(res.dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, dockerfileApp["Dockerfile"], string(dockerfile))
}

func TestLaunchNoCreateApp(t *testing.T) {
	env := newManifestEnv()
	res, out, err := env.launchFrom(t, dockerfileApp, "--no-create-app")
	require.NoError(t, err)

	assert.Empty(t, env.created, "no app created")
	assert.NotContains(t, out, "Created app")
	assert.Equal(t, "my-app", loadFlyToml(t, res.dir).AppName)
}

func TestLaunchConfigFormat(t *testing.T) {
	for _, tc := range []struct {
		flag string
		file string
	}{
		{flag: "--json", file: "fly.json"},
		{flag: "--yaml", file: "fly.yaml"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			res, _, err := newManifestEnv().launchFrom(t, dockerfileApp, tc.flag)
			require.NoError(t, err)

			assert.FileExists(t, filepath.Join(res.dir, tc.file))
			assert.NoFileExists(t, filepath.Join(res.dir, "fly.toml"))

			cfg, err := appconfig.LoadConfig(filepath.Join(res.dir, tc.file))
			require.NoError(t, err)
			assert.Equal(t, "my-app", cfg.AppName)
		})
	}
}

// Relaunching with --copy-config keeps what the user put in fly.toml that the
// plan doesn't know about.
func TestLaunchCopyConfigKeepsSettings(t *testing.T) {
	flyToml := `app = 'from-toml'
primary_region = 'syd'
kill_signal = 'SIGTERM'

[env]
  LOG_LEVEL = 'debug'

[http_service]
  internal_port = 4000
  auto_stop_machines = 'off'

[[vm]]
  memory = '2gb'
`
	env := newManifestEnv()
	files := map[string]string{"Dockerfile": "FROM nginx\n"}
	res, err := env.buildManifestWithFlyToml(t, files, flyToml, "--copy-config", "--no-deploy")
	require.NoError(t, err)

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(res.ctx, ios)
	state, err := stateFromManifest(ctx, *res.manifest, res.cache, &recoverableErrorBuilder{})
	require.NoError(t, err)
	require.NoError(t, state.Launch(ctx))

	require.Len(t, env.created, 1)
	assert.Equal(t, "from-toml", env.created[0].Name)

	cfg := loadFlyToml(t, res.dir)
	assert.Equal(t, "from-toml", cfg.AppName)
	assert.Equal(t, "syd", cfg.PrimaryRegion)
	assert.Equal(t, "SIGTERM", *cfg.KillSignal)
	assert.Equal(t, "debug", cfg.Env["LOG_LEVEL"])
	require.NotNil(t, cfg.HTTPService)
	assert.Equal(t, 4000, cfg.HTTPService.InternalPort)
	require.NotNil(t, cfg.HTTPService.AutoStopMachines)
	require.Len(t, cfg.Compute, 1)
	assert.Equal(t, "2gb", cfg.Compute[0].Memory)
}

func TestLaunchDockerignoreFromGitignore(t *testing.T) {
	files := map[string]string{
		"Dockerfile": "FROM nginx\n",
		".gitignore": "node_modules\n.env\n",
	}

	t.Run("--dockerignore-from-gitignore", func(t *testing.T) {
		res, _, err := newManifestEnv().launchFrom(t, files, "--dockerignore-from-gitignore")
		require.NoError(t, err)

		dockerignore, err := os.ReadFile(filepath.Join(res.dir, ".dockerignore"))
		require.NoError(t, err)
		assert.Contains(t, string(dockerignore), "node_modules")
		assert.Contains(t, string(dockerignore), ".env")
		// fly.toml holds nothing the image needs.
		assert.Contains(t, string(dockerignore), "fly.toml")
	})

	t.Run("headless without the flag", func(t *testing.T) {
		res, _, err := newManifestEnv().launchFrom(t, files)
		require.NoError(t, err)

		assert.NoFileExists(t, filepath.Join(res.dir, ".dockerignore"))
	})
}
