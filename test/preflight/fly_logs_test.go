//go:build integration
// +build integration

package preflight

import (
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/command/logs"
	"github.com/superfly/flyctl/test/preflight/testlib"
	"testing"
)

func TestFuncGetMachineId(t *testing.T) {
	// Test the function responsible for machine flag behavior

	// Test if clashing instance and machine flag values throw proper error
	t.Run("TestFuncThrowsErrorWhenInstanceAndMachineClash", func(tt *testing.T) {
		_, err := logs.GetMachineID("sampleInstanceId", "sampleMachineId")
		require.Contains(t, err.Error(), `--instance does not match the --machine`)
	})

	// Test if --machine works, by matching the machine argument(second argument) passed with result
	t.Run("TestFuncReturnsMachineIdIfProvided", func(tt *testing.T) {
		machineId, _ := logs.GetMachineID("", "sampleMachineId")
		require.Contains(t, machineId, "sampleMachineId")
	})

	// Test if --instance works, by matching the instance argument(first argument) passed with result
	t.Run("TestFuncReturnsInstanceIdIfProvided", func(tt *testing.T) {
		machineId, _ := logs.GetMachineID("sampleInstanceId", "")
		require.Contains(t, machineId, "sampleInstanceId")
	})
}

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

	// Test if clashing instance and machine flag values throw proper error
	t.Run("TestThrowsErrorWhenInstanceAndMachineClash", func(tt *testing.T) {
		res := f.FlyAllowExitFailure("logs --app "+appName+" --no-tail --instance " + machineId + " --machine instanceIdB")
		require.Contains(tt, res.StdErrString(), `--instance does not match the --machine`)
	})

	// Test if --machine works, should not throw an error
	t.Run("TestRunsWhenMachineIdProvided", func(tt *testing.T) {
		f.Fly("logs --app "+appName+" --no-tail --machine " + machineId)
	})

	// Test if --instance works, should not throw an error
	t.Run("TestRunsWhenInstanceIdProvided", func(tt *testing.T) {
		f.Fly("logs  --app "+appName+" --no-tail --instance " + machineId)
	})
}
