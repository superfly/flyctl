//go:build integration
// +build integration

package preflight

import (
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
	"testing"
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
