//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyLogsMachineFlagBehavior(t *testing.T) {
	// Test `flyctl logs` with different flag combinations

	// Get library from preflight test lib using env variables form  .direnv/preflight
	f := testlib.NewTestEnvFromEnv(t)
	if f.VMSize != "" {
		t.Skip()
	}

	// Create app, Create Machine, get Machine ID
	appName := f.CreateRandomAppMachines()
	f.Fly("machine run -a %s nginx --port 80:81 --autostop --region %s", appName, f.PrimaryRegion())
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))
	machineId := ml[0].ID

	// Test if --machine works, should not throw an error
	t.Run("TestRunsWhenMachineFlagProvided", func(tt *testing.T) {
		f.Fly("logs --app " + appName + " --no-tail --machine " + machineId)
	})

	// Test if --instance works, should not throw an error
	t.Run("TestRunsWhenInstanceFlagProvided", func(tt *testing.T) {
		f.Fly("logs  --app " + appName + " --no-tail --instance " + machineId)
	})

	// Test if alias shorthand -i works, should not throw an error
	t.Run("TestRunsWhenInstanceShorthandProvided", func(tt *testing.T) {
		f.Fly("logs --app " + appName + " --no-tail -i " + machineId)
	})
}

func TestFlyLogs(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node")
	require.NoError(t, err)

	toml := fmt.Sprintf("%s/fly.toml", f.WorkDir())
	app := f.CreateRandomAppName()

	err = testlib.OverwriteConfig(toml, map[string]any{
		"app":    app,
		"region": f.PrimaryRegion(),
		"env":    map[string]string{"TEST_ID": f.ID()},
	})
	require.NoError(t, err)

	f.Fly("launch --ha=false --copy-config --name %s --region %s --org %s --now", app, f.PrimaryRegion(), f.OrgSlug())

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", app))
	require.NoError(t, err)
	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))

	res := f.Fly("logs --app %s --no-tail", app)

	// The app logs the following message to stdout.
	require.Contains(t, res.StdOutString(), f.ID()+" is up\n")
}
