//go:build integration
// +build integration

package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyDeployBluegreenImplicitAppProcessGroup(t *testing.T) {
	t.Run("ManagedMachinesDoNotDuplicate", testFlyDeployBluegreenImplicitAppProcessGroupManagedMachines)
	t.Run("DetachedMachineIsRejected", testFlyDeployBluegreenImplicitAppProcessGroupDetachedMachine)
}

func testFlyDeployBluegreenImplicitAppProcessGroupManagedMachines(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	writeImplicitAppProcessFlyToml(f, appName, "one")

	deployBluegreenImplicitAppProcess(f)

	managedMachines, detachedMachines := requireMachineCounts(f, appName, 1, 0)
	require.Equal(f, fly.MachineProcessGroupApp, managedMachines[0].ProcessGroup())

	beforeID := managedMachines[0].ID

	runDetachedAppProcessMachine(f, appName)
	managedMachines, detachedMachines = requireMachineCounts(f, appName, 1, 1)
	require.Equal(f, beforeID, managedMachines[0].ID)
	detachedID := detachedMachines[0].ID

	writeImplicitAppProcessFlyToml(f, appName, "two")
	deployBluegreenImplicitAppProcess(f)

	managedMachines, detachedMachines = requireMachineCounts(f, appName, 1, 1)
	require.NotEqual(f, beforeID, managedMachines[0].ID)
	require.Equal(f, fly.MachineProcessGroupApp, managedMachines[0].ProcessGroup())
	require.Equal(f, detachedID, detachedMachines[0].ID)
}

func testFlyDeployBluegreenImplicitAppProcessGroupDetachedMachine(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	writeImplicitAppProcessFlyToml(f, appName, "one")

	runDetachedAppProcessMachine(f, appName)

	_, detachedMachines := requireMachineCounts(f, appName, 0, 1)
	require.Equal(f, fly.MachineProcessGroupApp, detachedMachines[0].ProcessGroup())

	res := f.FlyAllowExitFailure("deploy --buildkit --remote-only --now --image nginx --ha=false --strategy bluegreen")
	require.NotZero(f, res.ExitCode())
	require.Contains(f, res.StdErrString(), "outside Fly Launch management")
	require.Contains(f, res.StdErrString(), detachedMachines[0].ID)
}

func deployBluegreenImplicitAppProcess(f *testlib.FlyctlTestEnv) {
	f.Fly("deploy --buildkit --remote-only --now --image nginx --ha=false --strategy bluegreen")
}

func runDetachedAppProcessMachine(f *testlib.FlyctlTestEnv, appName string) {
	f.Fly(
		"m run -a %s -r %s --metadata %s=%s --env ENV=preflight -- nginx",
		appName,
		f.PrimaryRegion(),
		fly.MachineConfigMetadataKeyFlyProcessGroup,
		fly.MachineProcessGroupApp,
	)
}

func requireMachineCounts(f *testlib.FlyctlTestEnv, appName string, managedCount, detachedCount int) ([]*fly.Machine, []*fly.Machine) {
	var managedMachines, detachedMachines []*fly.Machine
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		managedMachines, detachedMachines = splitFlyLaunchMachines(f.MachinesList(appName))
		assert.Len(c, managedMachines, managedCount)
		assert.Len(c, detachedMachines, detachedCount)
	}, 2*time.Minute, 3*time.Second)

	return managedMachines, detachedMachines
}

func splitFlyLaunchMachines(machines []*fly.Machine) (managedMachines, detachedMachines []*fly.Machine) {
	for _, m := range machines {
		if m.Config != nil && m.Config.Metadata[fly.MachineConfigMetadataKeyFlyPlatformVersion] == fly.MachineFlyPlatformVersion2 {
			managedMachines = append(managedMachines, m)
			continue
		}

		detachedMachines = append(detachedMachines, m)
	}

	return managedMachines, detachedMachines
}

func writeImplicitAppProcessFlyToml(f *testlib.FlyctlTestEnv, appName, generation string) {
	f.WriteFlyToml(`app = "%s"
primary_region = "%s"

[env]
  PREFLIGHT_GENERATION = "%s"

[http_service]
  internal_port = 80
  force_https = true
  auto_stop_machines = "off"
  auto_start_machines = true
  min_machines_running = 1
  processes = ["app"]

  [[http_service.checks]]
    grace_period = "5s"
    interval = "10s"
    method = "GET"
    timeout = "2s"
    path = "/"
`, appName, f.PrimaryRegion(), generation)
}
