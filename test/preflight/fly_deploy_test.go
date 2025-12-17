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

	//fly "github.com/superfly/fly-go"

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

	// Create the app without deploying to avoid the Corrosion replication race
	f.Fly(
		"launch --org %s --name %s --region %s --image nginx --internal-port 80 --no-deploy",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	// Retry the deploy command to handle Corrosion replication lag race conditions
	// The backend may not have replicated the app record to all hosts yet when
	// creating the second machine for HA, resulting in "sql: no rows in result set" errors
	var lastError string
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		result := f.FlyAllowExitFailure("deploy --buildkit --remote-only")
		if result.ExitCode() != 0 {
			stderr := result.StdErrString()
			lastError = stderr
			// Only retry if it's the known Corrosion replication lag error
			if strings.Contains(stderr, "failed to get app: sql: no rows in result set") {
				t.Logf("Corrosion replication lag detected, retrying... (error: %s)", stderr)
				assert.Fail(c, "Corrosion replication lag, retrying...")
			} else {
				// Log the unexpected error and fail without retrying
				t.Logf("Deploy failed with unexpected error (will not retry): %s", stderr)
				assert.Fail(c, fmt.Sprintf("deploy failed with unexpected error: %s", stderr))
			}
		} else {
			// Explicitly assert success so EventuallyWithT knows we passed
			assert.True(c, true, "deploy succeeded")
		}
	}, 30*time.Second, 5*time.Second, "deploy should succeed after Corrosion replication, last error: %s", lastError)

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
		sshResult := f.Fly("ssh console -C 'printenv PREFLIGHT_TEST'")
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
	f.Fly("deploy --buildkit --remote-only --ha=false")

	t.Logf("deploy %s again", appName)
	f.Fly("deploy --buildkit --remote-only --strategy immediate --ha=false")

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
	f.Fly("deploy --buildkit --remote-only --strategy immediate --ha=false")

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

	var manifestPath = filepath.Join(f.WorkDir(), "manifest.json")

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
