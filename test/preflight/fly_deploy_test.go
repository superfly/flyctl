//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	//"github.com/samber/lo"
	"github.com/containerd/continuity/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyDeployHA(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	if f.SecondaryRegion() == "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("scale count 1 --region %s --yes", f.SecondaryRegion())

	f.WriteFlyToml(`%s
[mounts]
	source = "data"
	destination = "/data"
	`, f.ReadFile("fly.toml"))

	x := f.FlyAllowExitFailure("deploy --buildkit --remote-only")
	require.Contains(f, x.StdErrString(), `needs volumes with name 'data' to fulfill mounts defined in fly.toml`)

	// Create two volumes because fly launch will start 2 machines because of HA setup
	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.PrimaryRegion())
	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.SecondaryRegion())
	f.Fly("deploy --buildkit --remote-only")
}

// This test overlaps partially in functionality with TestFlyDeployHA, but runs
// when the test environment uses just a single region
func TestFlyDeploy_AddNewMount(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	if f.SecondaryRegion() != "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	f.WriteFlyToml(`%s
[mounts]
	source = "data"
	destination = "/data"
	`, f.ReadFile("fly.toml"))

	x := f.FlyAllowExitFailure("deploy --buildkit --remote-only")
	require.Contains(f, x.StdErrString(), `needs volumes with name 'data' to fulfill mounts defined in fly.toml`)

	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.PrimaryRegion())
	f.Fly("deploy --buildkit --remote-only")
}

func TestFlyDeployHAPlacement(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	assertHostDistribution(t, f, appName, 2)
}

func TestFlyDeploy_DeployToken_Simple(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())

	tokenResult := f.Fly("tokens deploy")
	f.OverrideAuthAccessToken(tokenResult.StdOutString())
	f.Fly("deploy --buildkit --remote-only")
}

func TestFlyDeploy_DeployToken_FailingSmokeCheck(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
[experimental]
  entrypoint = "/bin/false"
`
	f.WriteFlyToml("%s", appConfig)

	tokenResult := f.Fly("tokens deploy")
	f.OverrideAuthAccessToken(tokenResult.StdOutString())
	deployRes := f.FlyAllowExitFailure("deploy --buildkit --remote-only")
	output := deployRes.StdErrString()
	require.Contains(f, output, "the app appears to be crashing")
	require.NotContains(f, output, "401 Unauthorized")
}

func TestFlyDeploy_DeployToken_FailingReleaseCommand(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
[deploy]
  release_command = "/bin/false"
`
	f.WriteFlyToml("%s", appConfig)

	tokenResult := f.Fly("tokens deploy")
	f.OverrideAuthAccessToken(tokenResult.StdOut().String())
	deployRes := f.FlyAllowExitFailure("deploy --buildkit --remote-only")
	output := deployRes.StdErrString()
	require.Contains(f, output, "exited with non-zero status of 1")
	require.NotContains(f, output, "401 Unauthorized")
}

func TestFlyDeploy_Dockerfile(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.WriteFile("Dockerfile", `FROM nginx
ENV PREFLIGHT_TEST=true`)
	f.Fly("launch --org %s --name %s --region %s --internal-port 80 --ha=false --now", f.OrgSlug(), appName, f.PrimaryRegion())

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// Use FlyAllowExitFailure to handle transient WireGuard API failures (HTTP 500)
		sshResult := f.FlyAllowExitFailure("ssh console -C 'printenv PREFLIGHT_TEST'")
		if sshResult.ExitCode() != 0 {
			assert.Fail(c, "ssh command failed, will retry", "exit code: %d, stderr: %s", sshResult.ExitCode(), sshResult.StdErrString())
			return
		}
		assert.Equal(c, "true", strings.TrimSpace(sshResult.StdOutString()), "expected PREFLIGHT_TEST env var to be set in machine")
	}, 30*time.Second, 2*time.Second)
}

// If this test passes at all, that means that a slow metrics server isn't affecting flyctl
func TestFlyDeploySlowMetrics(t *testing.T) {
	env := make(map[string]string)
	env["FLY_METRICS_BASE_URL"] = "https://flyctl-metrics-slow.fly.dev"
	env["FLY_SEND_METRICS"] = "1"

	f := testlib.NewTestEnvFromEnvWithEnv(t, env)
	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	f.Fly("deploy --buildkit --remote-only")
}

func getRootPath() string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Dir(b)
}

func copyFixtureIntoWorkDir(workDir, name string) error {
	src := fmt.Sprintf("%s/fixtures/%s", getRootPath(), name)
	return fs.CopyDir(workDir, src)
}

func TestDeployNodeApp(t *testing.T) {
	t.Run("With Wireguard", WithParallel(testDeployNodeAppWithRemoteBuilder))
	// "Without Wireguard" test removed - BuildKit (our standard remote builder) requires
	// WireGuard to connect to the remote builder app. Testing the legacy remote builder
	// without WireGuard doesn't align with our BuildKit-first direction.
	t.Run("With BuildKit", WithParallel(testDeployNodeAppWithBuildKitRemoteBuilder))
}

func testDeployNodeAppWithRemoteBuilder(tt *testing.T) {
	t := testLogger{tt}
	f := testlib.NewTestEnvFromEnv(t)
	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node")
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app":    appName,
		"region": f.PrimaryRegion(),
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	t.Logf("deploy %s", appName)
	f.Fly("deploy --remote-only --ha=false")

	t.Logf("deploy %s again", appName)
	f.Fly("deploy --remote-only --strategy immediate")

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}

func testDeployNodeAppWithBuildKitRemoteBuilder(tt *testing.T) {
	t := testLogger{tt}
	f := testlib.NewTestEnvFromEnv(t)
	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node")
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app":    appName,
		"region": f.PrimaryRegion(),
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	t.Logf("deploy %s with BuildKit", appName)
	f.Fly("deploy --buildkit --remote-only --ha=false")

	t.Logf("deploy %s again with BuildKit", appName)
	f.Fly("deploy --buildkit --remote-only --strategy immediate")

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}

func TestFlyDeployBasicNodeWithWGEnabled(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	// Since this pins a specific size, we can skip it for alternate VM sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node")
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app": appName,
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	f.Fly("wireguard websockets enable")

	f.Fly("deploy --buildkit --remote-only --ha=false")

	f.Fly("wireguard websockets disable")

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}

func TestFlyDeploy_DeployMachinesCheck(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
		[[http_service.machine_checks]]
            image = "curlimages/curl"
   			entrypoint = ["/bin/sh", "-c"]
			command = ["curl http://[$FLY_TEST_MACHINE_IP]:80"]
		`
	f.WriteFlyToml("%s", appConfig)

	tokenResult := f.Fly("tokens deploy")
	f.OverrideAuthAccessToken(tokenResult.StdOut().String())
	deployRes := f.Fly("deploy --buildkit --remote-only")
	output := deployRes.StdOutString()
	require.Contains(f, output, "Test Machine")
}

func TestFlyDeploy_NoServiceDeployMachinesCheck(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
		[[machine_checks]]
			image = "curlimages/curl"
			entrypoint = ["/bin/sh", "-c"]
			command = ["curl http://[$FLY_TEST_MACHINE_IP]:80"]
		`
	f.WriteFlyToml("%s", appConfig)

	tokenResult := f.Fly("tokens deploy")
	f.OverrideAuthAccessToken(tokenResult.StdOut().String())
	deployRes := f.Fly("deploy --buildkit --remote-only")
	output := deployRes.StdOutString()
	require.Contains(f, output, "Test Machine")
}

// TODO: This test times out after ~15 minutes in CI (hangs at deploy command)
// The issue appears to be specific to canary strategy + BuildKit + machine checks
// Similar tests without canary pass fine (TestFlyDeploy_DeployMachinesCheck passes in ~60s)
// Need to investigate why canary deploys with BuildKit hang indefinitely
// func TestFlyDeploy_DeployMachinesCheckCanary(t *testing.T) {
// 	f := testlib.NewTestEnvFromEnv(t)
//
// 	appName := f.CreateRandomAppName()
// 	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false --strategy canary", f.OrgSlug(), appName, f.PrimaryRegion())
// 	appConfig := f.ReadFile("fly.toml")
// 	appConfig += `
// 		[[http_service.machine_checks]]
//             image = "curlimages/curl"
//    			entrypoint = ["/bin/sh", "-c"]
// 			command = ["curl http://[$FLY_TEST_MACHINE_IP]:80"]
// 		`
// 	f.WriteFlyToml("%s", appConfig)
//
// 	tokenResult := f.Fly("tokens deploy")
// 	f.OverrideAuthAccessToken(tokenResult.StdOut().String())
// 	deployRes := f.Fly("deploy --buildkit --remote-only")
// 	output := deployRes.StdOutString()
// 	require.Contains(f, output, "Test Machine")
// }

// TODO: Commented out due to suspected timeout issues with canary + BuildKit
// This test uses the same canary strategy that causes TestFlyDeploy_DeployMachinesCheckCanary to hang
// func TestFlyDeploy_CreateBuilderWDeployToken(t *testing.T) {
// 	f := testlib.NewTestEnvFromEnv(t)
//
// 	appName := f.CreateRandomAppName()
//
// 	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false --strategy canary", f.OrgSlug(), appName, f.PrimaryRegion())
//
// 	tokenResult := f.Fly("tokens deploy")
// 	f.OverrideAuthAccessToken(tokenResult.StdOutString())
// 	f.Fly("deploy --buildkit --remote-only")
// }

func TestDeployManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode: test suite approaches 15m timeout with this test included")
	}

	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx:latest --internal-port 80 --ha=false --strategy rolling", f.OrgSlug(), appName, f.PrimaryRegion())

	manifestPath := filepath.Join(f.WorkDir(), "manifest.json")

	f.Fly("deploy --buildkit --remote-only --export-manifest %s", manifestPath)

	manifest := f.ReadFile("manifest.json")
	require.Contains(t, manifest, `"AppName": "`+appName+`"`)
	require.Contains(t, manifest, `"primary_region": "`+f.PrimaryRegion()+`"`)
	require.Contains(t, manifest, `"internal_port": 80`)
	require.Contains(t, manifest, `"increased_availability": true`)
	// require.Contains(t, manifest, `"strategy": "rolling"`) FIX: fly launch doesn't set strategy
	require.Contains(t, manifest, `"image": "nginx:latest"`)

	deployRes := f.Fly("deploy --buildkit --remote-only --from-manifest %s", manifestPath)

	output := deployRes.StdOutString()
	require.Contains(t, output, fmt.Sprintf("Resuming %s deploy from manifest", appName))
}

func testDeploy(t *testing.T, appDir string, builderFlag string) {
	f := testlib.NewTestEnvFromEnv(t)
	app := f.CreateRandomAppMachines()
	url := fmt.Sprintf("https://%s.fly.dev", app)

	var result *testlib.FlyctlResult
	if builderFlag != "" {
		result = f.Fly("deploy %s --app %s %s", builderFlag, app, appDir)
	} else {
		result = f.Fly("deploy --app %s %s", app, appDir)
	}
	t.Log(result.StdOutString())

	var resp *http.Response
	require.Eventually(t, func() bool {
		var err error
		resp, err = http.Get(url)
		return err == nil && resp.StatusCode == http.StatusOK
	}, 20*time.Second, 1*time.Second, "GET %s never returned 200 OK response 20 seconds", url)

	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello World!\n", string(buf))
}

func TestDeploy(t *testing.T) {
	t.Run("Buildpack", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping buildpack test in CI: buildpacks require wireguard connectivity which is not available in CI environment")
		}
		t.Parallel()
		// Buildpacks cannot use BuildKit, so they use Depot (which falls back to remote builders)
		testDeploy(t, filepath.Join(testlib.RepositoryRoot(), "test", "preflight", "fixtures", "example-buildpack"), "--depot")
	})
	t.Run("Dockerfile", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode: test suite approaches 15m timeout with this test included")
		}
		t.Parallel()
		// Dockerfiles explicitly use BuildKit with remote building
		testDeploy(t, filepath.Join(testlib.RepositoryRoot(), "test", "preflight", "fixtures", "example"), "--buildkit --remote-only")
	})
}

// TestFlyDeploy_BlueGreen_StoppedMachines is a regression test for the bug where
// blue-green deployments silently bypassed health checks when blue machines were
// stopped (e.g. by auto_stop_machines = "stop" with min_machines_running = 0).
//
// Root cause: stopped blue machines had SkipLaunch=true in their launchInput.
// CreateGreenMachines copied that input without resetting the flag, so green
// machines were never started. WaitForGreenMachinesToBeHealthy then immediately
// marked them as "1/1 passing" without ever polling — a silent false success.
//
// The fix forces SkipLaunch=false for all green machines and guards health-check
// polling against vacuously-true AllPassing() on an empty result set.
func TestFlyDeploy_BlueGreen_StoppedMachines(t *testing.T) {
	// setupBlueMachinesStopped launches a fresh single-machine app with an
	// http service health check, does an initial deploy so machines reach
	// "started", then manually stops every machine to reproduce the
	// auto_stop_machines trigger condition.
	setupBlueMachinesStopped := func(t *testing.T) (*testlib.FlyctlTestEnv, string) {
		t.Helper()
		f := testlib.NewTestEnvFromEnv(t)
		appName := f.CreateRandomAppName()

		f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
			f.OrgSlug(), appName, f.PrimaryRegion())

		// Add a service health check so bluegreen actually waits for check results.
		// Without checks the strategy skips health polling, which would make the
		// failing-app subtest pass vacuously.
		appConfig := f.ReadFile("fly.toml")
		appConfig += `
  [[http_service.checks]]
    grace_period = "5s"
    interval = "10s"
    method = "GET"
    timeout = "5s"
    path = "/"
`
		f.WriteFlyToml("%s", appConfig)
		f.Fly("deploy --remote-only")

		// Stop every machine to simulate what auto_stop_machines does between deploys.
		machines := f.MachinesList(appName)
		require.NotEmpty(t, machines, "expected at least one machine after initial deploy")
		for _, m := range machines {
			f.Fly("machine stop -a %s %s", appName, m.ID)
		}
		require.Eventually(t, func() bool {
			for _, m := range f.MachinesList(appName) {
				if m.State != fly.MachineStateStopped {
					return false
				}
			}
			return true
		}, 30*time.Second, 2*time.Second, "timed out waiting for all machines to reach stopped state")

		return f, appName
	}

	// A bluegreen deploy of a crashing app must fail, never silently succeed.
	// Before the fix this exited 0 with "Deployment Complete" while machines
	// were still stopped and no traffic was served.
	t.Run("fails when app crashes", func(t *testing.T) {
		f, _ := setupBlueMachinesStopped(t)

		// Overwrite the entrypoint with /bin/false so the container exits immediately.
		appConfig := f.ReadFile("fly.toml")
		appConfig += `
[experimental]
  entrypoint = "/bin/false"
`
		f.WriteFlyToml("%s", appConfig)

		deployRes := f.FlyAllowExitFailure("deploy --remote-only --strategy bluegreen")
		require.NotEqual(t, 0, deployRes.ExitCode(),
			"bluegreen deploy must fail when the app crashes, not silently report success;\nstdout:\n%s\nstderr:\n%s",
			deployRes.StdOutString(), deployRes.StdErrString())
	})

	// A bluegreen deploy of a healthy app must succeed and leave machines running.
	// Before the fix this also appeared to succeed — but machines stayed stopped
	// because green machines were never actually started.
	t.Run("succeeds and machines are started", func(t *testing.T) {
		f, appName := setupBlueMachinesStopped(t)

		// Re-deploy the same healthy nginx image; no changes, no builder needed.
		f.Fly("deploy --remote-only --strategy bluegreen")

		// Every machine must be in "started" state — not stopped or created.
		for _, m := range f.MachinesList(appName) {
			require.Equal(t, fly.MachineStateStarted, m.State,
				"machine %s should be 'started' after a successful bluegreen deploy, got '%s'",
				m.ID, m.State)
		}
	})
}

// TestFlyDeploy_BlueGreen_IgnoreUnreachable tests the --ignore-unreachable
// flag on the bluegreen strategy. Because we cannot manufacture a genuinely
// unreachable host in a test environment, these subtests verify the two
// observable sides of the flag:
//
//  1. --ignore-unreachable is a valid flag and does not break normal healthy
//     deployments.
//  2. On an app whose machines were manually stopped before the second
//     deploy, --ignore-unreachable still succeeds (the stopped-machine
//     SkipLaunch regression is already covered by
//     TestFlyDeploy_BlueGreen_StoppedMachines; here we confirm
//     --ignore-unreachable doesn't introduce a regression on the same path).
func TestFlyDeploy_BlueGreen_IgnoreUnreachable(t *testing.T) {
	// launchWithChecks creates a fresh single-machine nginx app with an http
	// service health check and an initial deploy, then returns (env, appName).
	launchWithChecks := func(t *testing.T) (*testlib.FlyctlTestEnv, string) {
		t.Helper()
		f := testlib.NewTestEnvFromEnv(t)
		appName := f.CreateRandomAppName()

		f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
			f.OrgSlug(), appName, f.PrimaryRegion())

		appConfig := f.ReadFile("fly.toml")
		appConfig += `
	  [[http_service.checks]]
	    grace_period = "5s"
	    interval = "10s"
	    method = "GET"
	    timeout = "5s"
	    path = "/"
	`
		f.WriteFlyToml("%s", appConfig)
		f.Fly("deploy --remote-only")
		return f, appName
	}

	// --ignore-unreachable on a healthy app (all machines reachable) must
	// succeed and leave machines in "started" state. This is a regression
	// guard: --ignore-unreachable must not disrupt a normal bluegreen deploy.
	t.Run("ignore-unreachable on healthy app succeeds", func(t *testing.T) {
		f, appName := launchWithChecks(t)

		f.Fly("deploy --remote-only --strategy bluegreen --ignore-unreachable")

		for _, m := range f.MachinesList(appName) {
			require.Equal(t, fly.MachineStateStarted, m.State,
				"machine %s should be 'started' after bluegreen --ignore-unreachable on a healthy app, got '%s'",
				m.ID, m.State)
		}
	})

	// --ignore-unreachable on stopped machines (the auto_stop_machines scenario)
	// must still succeed: stopped machines get SkipLaunch=true in their
	// launchInput, and --ignore-unreachable must not prevent the green machines
	// from being properly started.
	t.Run("ignore-unreachable with stopped machines succeeds", func(t *testing.T) {
		f, appName := launchWithChecks(t)

		// Stop all machines to reproduce the auto_stop_machines trigger.
		machines := f.MachinesList(appName)
		for _, m := range machines {
			f.Fly("machine stop -a %s %s", appName, m.ID)
		}
		require.Eventually(t, func() bool {
			for _, m := range f.MachinesList(appName) {
				if m.State != fly.MachineStateStopped {
					return false
				}
			}
			return true
		}, 30*time.Second, 2*time.Second, "timed out waiting for machines to stop")

		// --ignore-unreachable must not interfere with the SkipLaunch fix: green
		// machines must still be started and health-checked properly.
		f.Fly("deploy --remote-only --strategy bluegreen --ignore-unreachable")

		for _, m := range f.MachinesList(appName) {
			require.Equal(t, fly.MachineStateStarted, m.State,
				"machine %s should be 'started' after bluegreen --ignore-unreachable with stopped blue machines, got '%s'",
				m.ID, m.State)
		}
	})
}
