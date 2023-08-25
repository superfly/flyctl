//go:build integration
// +build integration

package preflight

import (
	"strings"
	"testing"

	//"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	//"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyDeployHA(t *testing.T) {
	// t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --force-machines --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("scale count 1 --region %s --yes", f.SecondaryRegion())

	f.WriteFlyToml(`%s
[mounts]
	source = "data"
	destination = "/data"
	`, f.ReadFile("fly.toml"))

	x := f.FlyAllowExitFailure("deploy")
	require.Contains(f, x.StdErr().String(), `needs volumes with name 'data' to fullfill mounts defined in fly.toml`)

	// Create two volumes because fly launch will start 2 machines because of HA setup
	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.PrimaryRegion())
	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.SecondaryRegion())
	f.Fly("deploy")
}

func TestFlyDeploy_DeployToken_Simple(t *testing.T) {
	// t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --force-machines --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	f.OverrideAuthAccessToken(f.Fly("tokens deploy").StdOut().String())
	f.Fly("deploy")
}

func TestFlyDeploy_DeployToken_FailingSmokeCheck(t *testing.T) {
	// t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --force-machines --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
[experimental]
  entrypoint = "/bin/false"
`
	f.WriteFlyToml(appConfig)
	f.OverrideAuthAccessToken(f.Fly("tokens deploy").StdOut().String())
	deployRes := f.FlyAllowExitFailure("deploy")
	output := deployRes.StdErr().String()
	require.Contains(f, output, "the app appears to be crashing")
	require.NotContains(f, output, "401 Unauthorized")
}

func TestFlyDeploy_DeployToken_FailingReleaseCommand(t *testing.T) {
	// t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --force-machines --ha=false", f.OrgSlug(), appName, f.PrimaryRegion())
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
[deploy]
  release_command = "/bin/false"
`
	f.WriteFlyToml(appConfig)
	f.OverrideAuthAccessToken(f.Fly("tokens deploy").StdOut().String())
	deployRes := f.FlyAllowExitFailure("deploy")
	output := deployRes.StdErr().String()
	require.Contains(f, output, "exited with non-zero status of 1")
	require.NotContains(f, output, "401 Unauthorized")
}

func TestFlyDeploy_Dockerfile(t *testing.T) {
	// t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.WriteFile("Dockerfile", `FROM nginx
ENV PREFLIGHT_TEST=true`)
	f.Fly("launch --org %s --name %s --region %s --internal-port 80 --force-machines --ha=false --now", f.OrgSlug(), appName, f.PrimaryRegion())
	sshResult := f.Fly("ssh console -C 'printenv PREFLIGHT_TEST'")
	require.Equal(f, "true", strings.TrimSpace(sshResult.StdOut().String()), "expected PREFLIGHT_TEST env var to be set in machine")
}

// If this test passes at all, that means that a slow metrics server isn't affecting flyctl
func TestFlyDeploySlowMetrics(t *testing.T) {
	// t.Parallel()

	env := make(map[string]string)
	env["FLY_METRICS_BASE_URL"] = "https://flyctl-metrics-slow.fly.dev"
	env["FLY_SEND_METRICS"] = "1"

	f := testlib.NewTestEnvFromEnvWithEnv(t, env)
	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --force-machines --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	f.Fly("deploy")
}
