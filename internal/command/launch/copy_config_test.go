package launch

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
)

const copyConfigBase = `app = 'copied-app'
primary_region = 'iad'
`

const copyConfigWorker = copyConfigBase + `
[processes]
  worker = 'bin/worker'
`

const copyConfigLegacyService = copyConfigBase + `
[processes]
  server = 'bin/server'
  metrics = 'bin/metrics'

[[services]]
  processes = ['server']
  protocol = 'tcp'
  internal_port = 8000
  auto_stop_machines = 'suspend'
  auto_start_machines = true
  min_machines_running = 0
  [[services.ports]]
    port = 80
    handlers = ['http']
    force_https = true
  [[services.ports]]
    port = 443
    handlers = ['tls', 'http']
  [[services.tcp_checks]]
    interval = '15s'
    timeout = '2s'
    grace_period = '1s'
  [services.concurrency]
    type = 'requests'
    soft_limit = 20
    hard_limit = 25
`

const copyConfigMultipleServices = copyConfigLegacyService + `
[[services]]
  processes = ['metrics']
  protocol = 'udp'
  internal_port = 8125
  [[services.ports]]
    port = 8125
`

// Exercise real scanners, then reload the written config. Comparing complete
// service definitions catches dropped ports, process groups, checks and scaling
// settings as well as an unwanted [http_service].
func TestLaunchCopyConfigPreservesServices(t *testing.T) {
	for _, config := range []struct {
		name string
		toml string
	}{
		{name: "default process worker", toml: copyConfigBase},
		{name: "custom process worker", toml: copyConfigWorker},
		{name: "legacy service", toml: copyConfigLegacyService},
		{name: "multiple legacy services", toml: copyConfigMultipleServices},
	} {
		t.Run(config.name, func(t *testing.T) {
			for _, source := range []struct {
				name   string
				files  map[string]string
				config string
				family string
			}{
				{name: "EXPOSE disagrees with services", files: dockerfileApp, family: "Dockerfile"},
				{name: "no EXPOSE", files: map[string]string{"Dockerfile": "FROM nginx\n"}, family: "Dockerfile"},
				{name: "Containerfile", files: map[string]string{"Containerfile": "FROM nginx\nEXPOSE 9000\n"}, family: "Dockerfile"},
				{name: "Go source", files: goApp, family: "Go"},
				{name: "existing build", files: dockerfileApp, config: "\n[build]\n  image = 'nginx:alpine'\n"},
			} {
				t.Run(source.name, func(t *testing.T) {
					env := newManifestEnv()
					res, err := env.buildManifestWithFlyToml(t, source.files, config.toml+source.config, "--copy-config", "--no-deploy")
					require.NoError(t, err)
					original := loadFlyToml(t, res.dir)

					assert.Equal(t, source.family, res.manifest.Plan.ScannerFamily)
					assert.Zero(t, res.manifest.Plan.HttpServicePort)
					assert.False(t, res.manifest.Plan.HttpServicePortSetByScanner)
					if source.family != "" {
						require.NotNil(t, res.cache.sourceInfo)
						assert.Positive(t, res.cache.sourceInfo.Port, "exercise a scanner that would otherwise add an HTTP service")
					}

					_, err = launchPlan(t, res)
					require.NoError(t, err)

					cfg := loadFlyToml(t, res.dir)
					assert.Nil(t, cfg.HTTPService)
					assert.Equal(t, original.Services, cfg.Services)
					assert.Equal(t, original.Processes, cfg.Processes)
					if source.config != "" {
						assert.Equal(t, original.Build, cfg.Build)
					}
					require.NoError(t, cfg.SetMachinesPlatform())
					err, extra := cfg.Validate(res.ctx)
					assert.NoError(t, err, extra)
				})
			}
		})
	}
}

func TestLaunchCopyConfigServiceModes(t *testing.T) {
	for _, config := range []struct {
		name string
		toml string
	}{
		{name: "worker", toml: copyConfigWorker},
		{name: "legacy services", toml: copyConfigMultipleServices},
	} {
		t.Run(config.name, func(t *testing.T) {
			for _, mode := range []struct {
				name string
				args []string
			}{
				{name: "copy config", args: []string{"--copy-config"}},
				{name: "short config flag", args: []string{"-c", "fly.toml"}},
				{name: "long config flag", args: []string{"--config", "fly.toml"}},
				{name: "attach", args: []string{"--attach"}},
				{name: "noninteractive yes", args: []string{"--copy-config", "--yes"}},
				{name: "clone mode", args: []string{"--copy-config", "--now"}},
				{name: "explicit Dockerfile", args: []string{"--copy-config", "--dockerfile", "Dockerfile"}},
			} {
				t.Run(mode.name, func(t *testing.T) {
					// --no-create-app also prevents --now from deploying.
					args := append([]string{"--no-deploy", "--no-create-app"}, mode.args...)
					env := newManifestEnv()
					res, err := env.buildManifestWithFlyToml(t, dockerfileApp, config.toml, args...)
					require.NoError(t, err)
					original := loadFlyToml(t, res.dir)
					assert.Equal(t, 3000, res.cache.sourceInfo.Port)
					assert.Zero(t, res.manifest.Plan.HttpServicePort)
					assert.False(t, res.manifest.Plan.HttpServicePortSetByScanner)

					_, err = launchPlan(t, res)
					require.NoError(t, err)
					assert.Empty(t, env.created)
					cfg := loadFlyToml(t, res.dir)
					assert.Nil(t, cfg.HTTPService)
					assert.Equal(t, original.Services, cfg.Services)
					assert.Equal(t, original.Processes, cfg.Processes)
				})
			}
		})
	}
}

// An existing HTTP service must still receive scanner ports, retaining its
// process group and other settings. This also covers a zero scanner port when
// an existing build or --image bypasses source scanning.
func TestLaunchCopyConfigExistingHTTPService(t *testing.T) {
	const config = copyConfigMultipleServices + `
[http_service]
  internal_port = 4000
  processes = ['metrics']
  force_https = true
  auto_stop_machines = 'off'
  auto_start_machines = false
  min_machines_running = 1
  [http_service.concurrency]
    type = 'requests'
    soft_limit = 10
    hard_limit = 15
  [[http_service.checks]]
    interval = '15s'
    timeout = '2s'
    method = 'GET'
    path = '/health'
`
	for _, tc := range []struct {
		name     string
		files    map[string]string
		config   string
		args     []string
		wantPort int
		wantScan bool
	}{
		{name: "EXPOSE overrides port", files: dockerfileApp, wantPort: 3000, wantScan: true},
		{name: "no EXPOSE keeps port", files: map[string]string{"Dockerfile": "FROM nginx\n"}, wantPort: 4000, wantScan: true},
		{name: "Go scanner port", files: goApp, wantPort: 8080, wantScan: true},
		{name: "existing build keeps port", files: dockerfileApp, config: "\n[build]\n  image = 'nginx:alpine'\n", wantPort: 4000},
		{name: "image flag keeps port", files: dockerfileApp, args: []string{"--image", "nginx:alpine"}, wantPort: 4000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--copy-config", "--no-deploy"}, tc.args...)
			res, err := newManifestEnv().buildManifestWithFlyToml(t, tc.files, config+tc.config, args...)
			require.NoError(t, err)
			original := loadFlyToml(t, res.dir)
			require.NotNil(t, original.HTTPService)
			assert.Equal(t, tc.wantPort, res.manifest.Plan.HttpServicePort)
			assert.Equal(t, tc.wantScan, res.manifest.Plan.HttpServicePortSetByScanner)

			_, err = launchPlan(t, res)
			require.NoError(t, err)
			cfg := loadFlyToml(t, res.dir)
			original.HTTPService.InternalPort = tc.wantPort
			assert.Equal(t, original.HTTPService, cfg.HTTPService)
			assert.Equal(t, original.Services, cfg.Services)
			assert.Equal(t, original.Processes, cfg.Processes)
		})
	}
}

func TestLaunchCopyConfigManifestWithoutCache(t *testing.T) {
	for _, tc := range []struct {
		name string
		toml string
	}{
		{name: "worker", toml: copyConfigWorker},
		{name: "legacy services", toml: copyConfigMultipleServices},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := newManifestEnv().buildManifestWithFlyToml(t, dockerfileApp, tc.toml, "--copy-config", "--no-deploy")
			require.NoError(t, err)
			original := loadFlyToml(t, res.dir)
			data, err := json.Marshal(res.manifest)
			require.NoError(t, err)
			var loaded LaunchManifest
			require.NoError(t, json.Unmarshal(data, &loaded))
			assert.Zero(t, loaded.Plan.HttpServicePort)
			assert.False(t, loaded.Plan.HttpServicePortSetByScanner)

			// A saved manifest has no planning cache. Reload the original
			// config independently, then force stateFromManifest to rescan.
			res.ctx = appconfig.WithConfig(res.ctx, loadFlyToml(t, res.dir))
			state, err := stateFromManifest(res.ctx, loaded, nil, &recoverableErrorBuilder{})
			require.NoError(t, err)
			require.NotNil(t, state.sourceInfo)
			assert.Equal(t, 3000, state.sourceInfo.Port)
			require.NoError(t, state.Launch(res.ctx))
			cfg := loadFlyToml(t, res.dir)
			assert.Nil(t, cfg.HTTPService)
			assert.Equal(t, original.Services, cfg.Services)
			assert.Equal(t, original.Processes, cfg.Processes)
		})
	}
}

func TestLaunchCopyConfigFalseCreatesHTTPService(t *testing.T) {
	res, err := newManifestEnv().buildManifestWithFlyToml(t, dockerfileApp, copyConfigMultipleServices, "--copy-config=false", "--no-deploy")
	require.NoError(t, err)
	assert.Equal(t, 3000, res.manifest.Plan.HttpServicePort)
	assert.True(t, res.manifest.Plan.HttpServicePortSetByScanner)

	_, err = launchPlan(t, res)
	require.NoError(t, err)
	cfg := loadFlyToml(t, res.dir)
	require.NotNil(t, cfg.HTTPService)
	assert.Equal(t, 3000, cfg.HTTPService.InternalPort)
	assert.Equal(t, []string{"app"}, cfg.HTTPService.Processes)
	assert.Empty(t, cfg.Services)
	assert.Empty(t, cfg.Processes)
}
