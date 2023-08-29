//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

// TODO: list of things to test
// - sourceInfo clone vs launch modes
// - sourceInfo remote dockerfile vs local
// END

// Launch a new app and iterate rerunning `fly launch` to reuse the same app name and config
//
// - Create a V2 app
// - Must contain [http_service] section (no [[services]])
// - primary_region set and updated on subsequent 'fly launch --region other' calls
// - Internal port is set in first call and not replaced unless --internal-port is passed again
// - Primary region found in imported fly.toml must be reused if set and no --region is passed
// - As we are reusing an existing app, the --org param is not needed after the first call
func TestFlyLaunchV2(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --no-deploy --org %s --name %s --region %s --image nginx --force-machines", f.OrgSlug(), appName, f.PrimaryRegion())
	toml := f.UnmarshalFlyToml()
	want := map[string]any{
		"app":            appName,
		"primary_region": f.PrimaryRegion(),
		"build":          map[string]any{"image": "nginx"},
		"http_service": map[string]any{
			"force_https":          true,
			"internal_port":        int64(8080),
			"auto_stop_machines":   true,
			"auto_start_machines":  true,
			"min_machines_running": int64(0),
			"processes":            []any{"app"},
		},
	}
	require.EqualValues(f, want, toml)

	f.Fly("launch --no-deploy --reuse-app --copy-config --name %s --region %s --image nginx:stable", appName, f.SecondaryRegion())
	toml = f.UnmarshalFlyToml()
	want["primary_region"] = f.SecondaryRegion()
	if build, ok := want["build"].(map[string]any); true {
		require.True(f, ok)
		build["image"] = "nginx:stable"
	}
	require.Equal(f, want, toml)

	f.Fly("launch --no-deploy --reuse-app --copy-config --name %s --image nginx:stable --internal-port 9999", appName)
	toml = f.UnmarshalFlyToml()
	if service, ok := want["http_service"].(map[string]any); true {
		require.True(f, ok)
		service["internal_port"] = int64(9999)
	}
	require.EqualValues(f, want, toml)
}

// Same as case01 but for Nomad apps
func TestFlyLaunchV1(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --no-deploy --org %s --name %s --region %s --image nginx --internal-port 80 --force-nomad", f.OrgSlug(), appName, f.PrimaryRegion())
	toml := f.UnmarshalFlyToml()
	want := map[string]any{
		"app":            appName,
		"build":          map[string]any{"image": "nginx"},
		"env":            map[string]any{},
		"experimental":   map[string]any{"auto_rollback": true},
		"kill_signal":    "SIGINT",
		"kill_timeout":   int64(5),
		"primary_region": f.PrimaryRegion(),
		"processes":      []any{},
		"services": []map[string]any{{
			"concurrency":   map[string]any{"hard_limit": int64(25), "soft_limit": int64(20), "type": "connections"},
			"http_checks":   []any{},
			"internal_port": int64(80),
			"ports": []map[string]any{
				{"force_https": true, "handlers": []any{"http"}, "port": int64(80)},
				{"handlers": []any{"tls", "http"}, "port": int64(443)},
			},
			"processes":     []any{"app"},
			"protocol":      "tcp",
			"script_checks": []any{},
			"tcp_checks": []map[string]any{{
				"grace_period":  "1s",
				"interval":      "15s",
				"timeout":       "2s",
				"restart_limit": int64(0),
			}},
		}},
	}
	require.EqualValues(f, want, toml)

	f.Fly("launch --no-deploy --reuse-app --copy-config --name %s --region %s --image nginx:stable --force-nomad", appName, f.SecondaryRegion())
	toml = f.UnmarshalFlyToml()
	want["primary_region"] = f.SecondaryRegion()
	if build, ok := want["build"].(map[string]any); true {
		require.True(f, ok)
		build["image"] = "nginx:stable"
	}
	require.EqualValues(f, want, toml)

	f.Fly("launch --no-deploy --reuse-app --copy-config --name %s --image nginx:stable --internal-port 9999 --force-nomad", appName)
	toml = f.UnmarshalFlyToml()
	if services, ok := want["services"].([]map[string]any); true {
		require.True(f, ok)
		services[0]["internal_port"] = int64(9999)
	}
	require.EqualValues(f, want, toml)
}

// Run fly launch from a template Fly App directory (fly.toml without app name)
func TestFlyLaunchWithTOML(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.WriteFlyToml(`
[build]
  image = "superfly/postgres:15"

[checks.status]
	type = "tcp"
	port = 5500
	`)

	f.Fly("launch --no-deploy --org %s --name %s --region %s --force-machines --copy-config", f.OrgSlug(), appName, f.PrimaryRegion())
	toml := f.UnmarshalFlyToml()
	want := map[string]any{
		"app":            appName,
		"primary_region": f.PrimaryRegion(),
		"build":          map[string]any{"image": "superfly/postgres:15"},
		"checks": map[string]any{
			"status": map[string]any{"type": "tcp", "port": int64(5500)},
		},
	}
	require.EqualValues(f, want, toml)

	// reuse the config and app but update the image
	f.Fly("launch --no-deploy --reuse-app --copy-config --name %s --image superfly/postgres:14", appName)
	toml = f.UnmarshalFlyToml()
	if build, ok := want["build"].(map[string]any); true {
		require.True(f, ok)
		build["image"] = "superfly/postgres:14"
	}
	require.EqualValues(f, want, toml)
}

// Trying to import an invalid fly.toml should fail before creating the app
func TestFlyLaunchWithInvalidTOML(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.WriteFlyToml(`
app = "foo"

[[services]]
	internal_port = "can'tparse as port number"  # invalid
	protocol = "tcp"
	`)

	x := f.FlyAllowExitFailure("launch --no-deploy --org %s --name %s --region %s --force-machines --copy-config", f.OrgSlug(), appName, f.PrimaryRegion())
	require.Contains(f, x.StdErrString(), `Can not use configuration for Apps V2, check fly.toml`)
}

// Fail if the existing app doesn't match the forced platform version
// V2 app forced as V1
func TestFlyLaunchForceV1(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.Fly("apps create %s --machines -o %s", appName, f.OrgSlug())
	x := f.FlyAllowExitFailure("launch --no-deploy --reuse-app --name %s --region %s --force-nomad", appName, f.PrimaryRegion())
	require.Contains(f, x.StdErrString(), `--force-nomad won't work for existing app in machines platform`)
}

// test --generate-name, --name and reuse imported name
func TestFlyLaunchReuseName(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	// V2 app forced as V1
	appName1 := f.CreateRandomAppName()
	appName2 := f.CreateRandomAppName()
	appName3 := f.CreateRandomAppName()

	f.WriteFlyToml(`
app = "%s"
primary_region = "%s"
`, appName1, f.PrimaryRegion())

	f.Fly("launch --no-deploy --copy-config --name %s -o %s", appName2, f.OrgSlug())
	toml := f.UnmarshalFlyToml()
	require.EqualValues(f, appName2, toml["app"])

	f.Fly("launch --no-deploy --copy-config=false --name %s --region %s -o %s", appName3, f.PrimaryRegion(), f.OrgSlug())
	toml = f.UnmarshalFlyToml()
	require.EqualValues(f, appName3, toml["app"])

	f.Fly("launch --no-deploy --copy-config --generate-name -o %s", f.OrgSlug())
	toml = f.UnmarshalFlyToml()
	generatedName := toml["app"]
	f.Cleanup(func() {
		f.Fly("apps destroy -y %s", generatedName)
	})
	require.NotEqual(f, appName3, toml["app"])
}

// test volumes are created on first launch
func TestFlyLaunchWithVolumes(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.WriteFlyToml(`
[build]
  image = "nginx"

[processes]
	app = ""
	other = "sleep inf"
	backend = "sleep 1h"

[[mounts]]
  source = "data"
	destination = "/data"
	processes = ["app"]

[[mounts]]
  source = "trashbin"
	destination = "/data"
	processes = ["other"]
`)

	f.Fly("launch --now --copy-config -o %s --name %s --region %s --force-machines", f.OrgSlug(), appName, f.PrimaryRegion())
}

// test --vm-size sets the machine guest on first deploy
func TestFlyLaunchWithSize(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --ha=false --now -o %s --name %s --region %s --force-machines --image nginx --vm-size shared-cpu-4x",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))
	require.Equal(f, "shared-cpu-4x", ml[0].Config.Guest.ToSize())
}

// test default HA setup
func TestFlyLaunchHA(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.WriteFlyToml(`
[build]
  image = "nginx"

[processes]
	app = ""
	task = "sleep inf"
	disk = "sleep 1h"

[[mounts]]
  source = "disk"
	destination = "/data"
	processes = ["disk"]

[http_service]
	internal_port = 80
	auto_start_machines = true
	auto_stop_machines = true
	processes = ["app"]
`)

	f.Fly("launch --now --copy-config -o %s --name %s --region %s --force-machines", f.OrgSlug(), appName, f.PrimaryRegion())

	var ml []*api.Machine

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml = f.MachinesList(appName)
		assert.Equal(c, 5, len(ml), "want 5 machines, which includes two standbys")
	}, 10*time.Second, 1*time.Second)

	groups := lo.GroupBy(ml, func(m *api.Machine) string {
		return m.ProcessGroup()
	})

	require.Equal(f, 3, len(groups))
	require.Equal(f, 2, len(groups["app"]))
	require.Equal(f, 2, len(groups["task"]))
	require.Equal(f, 1, len(groups["disk"]))

	isStandby := func(m *api.Machine) bool { return len(m.Config.Standbys) > 0 }

	require.Equal(f, 0, lo.CountBy(groups["app"], isStandby))
	require.Equal(f, 1, lo.CountBy(groups["task"], isStandby))
	require.Equal(f, 0, lo.CountBy(groups["disk"], isStandby))

	hasServices := func(m *api.Machine) bool { return len(m.Config.Services) > 0 }

	require.Equal(f, 2, lo.CountBy(groups["app"], hasServices))
	require.Equal(f, 0, lo.CountBy(groups["task"], hasServices))
	require.Equal(f, 0, lo.CountBy(groups["disk"], hasServices))
}

// test first deploy with single mount for multiple processes
func TestFlyLaunchSingleMount(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.WriteFlyToml(`
[build]
  image = "nginx"

[processes]
	app = ""
	task = ""

[[mounts]]
  source = "data"
	destination = "/data"
	processes = ["app", "task"]
`)

	f.Fly("launch --now --copy-config -o %s --name %s --region %s --force-machines", f.OrgSlug(), appName, f.PrimaryRegion())
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml := f.MachinesList(appName)
		assert.Equal(c, 2, len(ml))
	}, 15*time.Second, 1*time.Second, "want 2 machines, one for each process")

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		assert.Equal(c, 2, len(vl))
	}, 15*time.Second, 1*time.Second, "want 2 volumes, one for each process")
}

func TestFlyLaunchWithBuildSecrets(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	// secrets are mounted under /run/secrets by default.
	// https://docs.docker.com/engine/reference/builder/#run---mounttypesecret
	f.WriteFile("Dockerfile", `FROM nginx
RUN --mount=type=secret,id=secret1 cat /run/secrets/secret1 > /tmp/secrets.txt
`)

	f.Fly("launch --org %s --name %s --region %s --internal-port 80 --force-machines --ha=false --now --build-secret secret1=SECRET1 --remote-only", f.OrgSlug(), appName, f.PrimaryRegion())

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ssh := f.Fly("ssh console -C 'cat /tmp/secrets.txt'")
		assert.Equal(c, "SECRET1", ssh.StdOut().String())
	}, 10*time.Second, 1*time.Second)
}

func TestFlyLaunchBasicNodeApp(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node", []string{})
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppName()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app": appName,
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	f.Fly("launch --ha=false --copy-config --name %s --region %s --org %s --now", appName, f.PrimaryRegion(), f.OrgSlug())

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)
	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}
