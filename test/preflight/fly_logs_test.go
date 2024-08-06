//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/command/logs"
	"github.com/superfly/flyctl/test/preflight/testlib"
	"strings"
	"testing"
)

func TestFuncGetMachineId(t *testing.T) {
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

func TestFlyLogsBehavior(t *testing.T) {

	// Get library from preflight test lib using env variables form  .direnv/preflight
	f := testlib.NewTestEnvFromEnv(t)
	if f.VMSize != "" {
		t.Skip()
	}

	// Create fly.toml in current directory for the purpose of getting instance id that will make flyctl run properly
	appName := f.CreateRandomAppName()
	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	// Try to get an instance id from running `fly machines list`
	machineListResult := f.Fly("machines list").StdOutString()
	instanceId, err := getInstanceIdFromListOutput(machineListResult)

	if instanceId != "" && err == nil {
		// Test `flyctl logs` with different flag combinations

		// Test if clashing instance and machine flag values throw proper error
		t.Run("TestThrowsErrorWhenInstanceAndMachineClash", func(tt *testing.T) {
			res := f.FlyAllowExitFailure("logs --no-tail --instance " + instanceId + " --machine instanceIdB")
			require.Contains(tt, res.StdErrString(), `--instance does not match the --machine`)
		})

		// Test if --machine works, should not throw an error
		t.Run("TestRunsWhenMachineIdProvided", func(tt *testing.T) {
			f.Fly("logs --no-tail --machine " + instanceId)
		})

		// Test if --instance works, should not throw an error
		t.Run("TestRunsWhenInstanceIdProvided", func(tt *testing.T) {
			f.Fly("logs --no-tail --instance " + instanceId)
		})
	}
}

func getInstanceIdFromListOutput(machineListResultStr string) (string, error) {
	// This function aims to get the instance id from the output of fly machine list

	// SIZE is the substring before the first instance id
	res := strings.Split(machineListResultStr, "SIZE")
	if len(res) > 1 {
		// Get the first item separated by space
		res2 := strings.Fields(res[1])
		if len(res2) > 1 {
			// First item in list will be the instance id
			return res2[0], nil
		}
	}
	return "", fmt.Errorf("Unable to retrieve the instance id from the output of fly machine list")
}
