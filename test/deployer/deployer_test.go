//go:build integration
// +build integration

package deployer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/testlib"
)

func TestDeployBasicNode(t *testing.T) {
	deploy := testDeployer(t,
		withFixtureApp("deploy-node"),
		createRandomApp,
		withOverwrittenConfig(func(d *testlib.DeployTestRun) map[string]any {
			return map[string]any{
				"app":    d.Extra["appName"],
				"region": d.PrimaryRegion(),
				"env": map[string]string{
					"TEST_ID": d.ID(),
				},
			}
		}),
		testlib.DeployOnly,
		testlib.DeployNow,
		withWorkDirAppSource,
	)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", deploy.Extra["appName"].(string)))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", deploy.Extra["TEST_ID"].(string)))
}

func TestDeployBasicNodeWithCustomConfigPath(t *testing.T) {
	deploy := testDeployer(t,
		withCustomFlyTomlPath("custom-fly-config.toml"),
		withFixtureApp("deploy-node-custom-config-path"),
		createRandomApp,
		withOverwrittenConfig(func(d *testlib.DeployTestRun) map[string]any {
			return map[string]any{
				"app":    d.Extra["appName"],
				"region": d.PrimaryRegion(),
				"env": map[string]string{
					"TEST_ID": d.ID(),
				},
			}
		}),
		testlib.DeployOnly,
		testlib.DeployNow,
		withWorkDirAppSource,
	)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", deploy.Extra["appName"].(string)))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", deploy.Extra["TEST_ID"].(string)))
}

func TestDeployBasicNodeMonorepo(t *testing.T) {
	deploy := testDeployer(t,
		withCustomCwd("inner-repo"),
		withFixtureApp("deploy-node-monorepo"),
		createRandomApp,
		withOverwrittenConfig(func(d *testlib.DeployTestRun) map[string]any {
			return map[string]any{
				"app":    d.Extra["appName"],
				"region": d.PrimaryRegion(),
				"env": map[string]string{
					"TEST_ID": d.ID(),
				},
			}
		}),
		testlib.DeployOnly,
		testlib.DeployNow,
		withWorkDirAppSource,
	)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", deploy.Extra["appName"].(string)))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", deploy.Extra["TEST_ID"].(string)))
}

func TestLaunchBasicNodeWithDockerfile(t *testing.T) {
	deploy := testDeployer(t,
		withFixtureApp("deploy-node"),
		withOverwrittenConfig(func(d *testlib.DeployTestRun) map[string]any {
			return map[string]any{
				"app":    "dummy-app-name",
				"region": d.PrimaryRegion(),
				"env": map[string]string{
					"TEST_ID": d.ID(),
				},
			}
		}),
		createRandomApp,
		testlib.WithCopyConfig,
		testlib.WithoutCustomize,
		testlib.WithouExtensions,
		testlib.DeployNow,
		withWorkDirAppSource,
	)

	appName := deploy.Extra["appName"].(string)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", deploy.Extra["TEST_ID"].(string)))
}

func TestLaunchBasicNode(t *testing.T) {
	deploy := testDeployer(t,
		withFixtureApp("deploy-node-no-dockerfile"),
		createRandomApp,
		testlib.WithoutCustomize,
		testlib.WithouExtensions,
		testlib.OptOutGithubActions,
		testlib.DeployNow,
		withWorkDirAppSource,
	)

	manifest, err := deploy.Output().ArtifactManifest()
	require.NoError(t, err)
	require.NotNil(t, manifest)

	require.Equal(t, manifest.Plan.Runtime.Language, "node")

	appName := deploy.Extra["appName"].(string)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Equal(t, string(body), "Hello, World!")
}

func TestLaunchGoFromRepo(t *testing.T) {
	deploy := testDeployer(t,
		createRandomApp,
		testlib.WithRegion("yyz"),
		testlib.WithoutCustomize,
		testlib.WithouExtensions,
		testlib.DeployNow,
		testlib.WithGitRepo("https://github.com/fly-apps/go-example"),
	)

	appName := deploy.Extra["appName"].(string)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), "I'm running in the yyz region")
}

func TestLaunchRails70(t *testing.T) {
	deploy := testDeployer(t,
		withFixtureApp("deploy-rails-7.0"),
		createRandomApp,
		testlib.WithoutCustomize,
		testlib.WithouExtensions,
		testlib.DeployNow,
		withWorkDirAppSource,
		testlib.CleanupBeforeExit,
	)

	manifest, err := deploy.Output().ArtifactManifest()
	require.NoError(t, err)
	require.NotNil(t, manifest)

	require.Equal(t, "ruby", manifest.Plan.Runtime.Language)

	appName := deploy.Extra["appName"].(string)

	_, err = testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev/up", appName))
	require.NoError(t, err)
}

func TestLaunchRails72(t *testing.T) {
	deploy := testDeployer(t,
		withFixtureApp("deploy-rails-7.2"),
		createRandomApp,
		testlib.WithoutCustomize,
		testlib.WithouExtensions,
		testlib.DeployNow,
		withWorkDirAppSource,
		testlib.CleanupBeforeExit,
	)

	manifest, err := deploy.Output().ArtifactManifest()
	require.NoError(t, err)
	require.NotNil(t, manifest)

	require.Equal(t, "ruby", manifest.Plan.Runtime.Language)

	appName := deploy.Extra["appName"].(string)

	_, err = testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev/up", appName))
	require.NoError(t, err)
}

func TestLaunchRails8(t *testing.T) {
	deploy := testDeployer(t,
		withFixtureApp("deploy-rails-8"),
		createRandomApp,
		testlib.WithoutCustomize,
		testlib.WithouExtensions,
		testlib.DeployNow,
		withWorkDirAppSource,
		testlib.CleanupBeforeExit,
	)

	manifest, err := deploy.Output().ArtifactManifest()
	require.NoError(t, err)
	require.NotNil(t, manifest)

	require.Equal(t, "ruby", manifest.Plan.Runtime.Language)
	require.Equal(t, "Rails", manifest.Plan.ScannerFamily)

	appName := deploy.Extra["appName"].(string)

	_, err = testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev/up", appName))
	require.NoError(t, err)
}

func createRandomApp(d *testlib.DeployTestRun) {
	appName := d.CreateRandomAppName()
	require.NotEmpty(d, appName)

	d.Fly("apps create %s -o %s", appName, d.OrgSlug())
	d.Extra["appName"] = appName

	testlib.WithApp(appName)(d)
}

func withFixtureApp(name string) func(*testlib.DeployTestRun) {
	return func(d *testlib.DeployTestRun) {
		err := testlib.CopyFixtureIntoWorkDir(d.WorkDir(), name)
		require.NoError(d, err)
	}
}

func withCustomFlyTomlPath(name string) func(*testlib.DeployTestRun) {
	return func(d *testlib.DeployTestRun) {
		d.FlyTomlPath = name
	}
}

func withCustomCwd(name string) func(*testlib.DeployTestRun) {
	return func(d *testlib.DeployTestRun) {
		d.Cwd = name
	}
}

func withOverwrittenConfig(raw any) func(*testlib.DeployTestRun) {
	return func(d *testlib.DeployTestRun) {
		flyTomlPath := d.WorkDir()
		if d.Cwd != "" {
			flyTomlPath = fmt.Sprintf("%s/%s", flyTomlPath, d.Cwd)
		}
		flyTomlPath = fmt.Sprintf("%s/%s", flyTomlPath, d.FlyTomlPath)
		data := make(map[string]any)
		switch cast := raw.(type) {
		case map[string]any:
			data = cast
		case func(*testlib.DeployTestRun) map[string]any:
			data = cast(d)
		default:
			fmt.Println(cast)
			d.Fatal("failed to cast template data")
		}
		err := testlib.OverwriteConfig(flyTomlPath, data)
		require.NoError(d, err)
	}
}

func withWorkDirAppSource(d *testlib.DeployTestRun) {
	testlib.WithAppSource(d.WorkDir())(d)
}

func testDeployer(t *testing.T, options ...func(*testlib.DeployTestRun)) *testlib.DeployTestRun {
	ctx := context.TODO()

	d, err := testlib.NewDeployerTestEnvFromEnv(ctx, t)
	require.NoError(t, err)

	defer d.Close()

	deploy := d.NewRun(options...)
	defer deploy.Close()

	deploy.Extra["TEST_ID"] = d.ID()

	err = deploy.Start(ctx)

	require.Nil(t, err)

	err = deploy.Wait()
	require.Nil(t, err)

	require.Zero(t, deploy.ExitCode())

	out := deploy.Output()

	meta, err := out.ArtifactMeta()
	require.NoError(t, err)

	stepNames := append([]string{"__root__"}, meta.StepNames()...)

	require.Equal(t, out.Steps, stepNames)

	return deploy
}
