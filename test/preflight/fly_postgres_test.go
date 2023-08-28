//go:build integration
// +build integration

package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestPostgres_singleNode(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	f.Fly("config validate")
}

func TestPostgres_autostart(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]

	for _, svc := range firstMachine.Config.Services {
		require.NotNil(t, svc.Autostart, "autostart when not set defaults to True")
		require.False(t, *svc.Autostart, "autostart wasn't disabled")
	}

	appName = f.CreateRandomAppName()
	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --autostart", f.OrgSlug(), appName, f.PrimaryRegion())
	machList = f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine = machList[0]
	for _, svc := range firstMachine.Config.Services {
		// Autostart defaults to True if not set
		if svc.Autostart != nil {
			require.True(t, *svc.Autostart, "autostart wasn't disabled")
		}
	}
}

func TestPostgres_FlexFailover(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	findLeaderID := func(ml []*api.Machine) string {
		for _, mach := range ml {
			for _, chk := range mach.Checks {
				if chk.Name == "role" && chk.Output == "primary" {
					return mach.ID
				}
			}
		}
		return ""
	}

	f.Fly("pg create --flex --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	leaderMachineID := findLeaderID(machList)
	if leaderMachineID == "" {
		f.Fatalf("Failed to find PG cluster leader")
	}

	f.Fly("pg failover -a %s", appName)
	machList = f.MachinesList(appName)
	newLeaderMachineID := findLeaderID(machList)
	require.NotEqual(t, newLeaderMachineID, leaderMachineID, "Failover failed! PG Leader didn't change!")
}

func TestPostgres_NoMachines(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]
	f.Fly("m destroy %s -a %s --force", firstMachine.ID, appName)
	result := f.Fly("status -a %s", appName)

	require.Contains(f, result.StdOut().String(), "No machines are available on this app")
}

func TestPostgres_haConfigSave(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 3, len(ml))
	require.Equal(f, "shared-cpu-1x", ml[0].Config.Guest.ToSize())
	f.Fly("config validate")
}

func TestPostgres_ImportSuccess(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	firstAppName := f.CreateRandomAppName()
	secondAppName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --password x",
		f.OrgSlug(), firstAppName, f.PrimaryRegion(),
	)
	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), secondAppName, f.PrimaryRegion(),
	)

	f.Fly(
		"ssh console -a %s -u postgres -C \"psql -p 5433 -h /run/postgresql -c 'CREATE TABLE app_name (app_name TEXT)'\"",
		firstAppName,
	)
	f.Fly(
		"ssh console -a %s -u postgres -C \"psql -p 5433 -h /run/postgresql -c \\\"INSERT INTO app_name VALUES ('%s')\\\"\"",
		firstAppName, firstAppName,
	)

	f.Fly(
		"pg import -a %s --region %s --vm-size shared-cpu-1x postgres://postgres:x@%s.internal/postgres",
		secondAppName, f.PrimaryRegion(), firstAppName,
	)

	result := f.Fly(
		"ssh console -a %s -u postgres -C \"psql -p 5433 -h /run/postgresql -c 'SELECT app_name FROM app_name'\"",
		secondAppName,
	)
	output := result.StdOut().String()
	require.Contains(f, output, firstAppName)

	// The importer machine should have been destroyed.
	ml := f.MachinesList(secondAppName)
	require.Equal(f, 1, len(ml))
}

func TestPostgres_ImportFailure(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --password x",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	result := f.FlyAllowExitFailure(
		"pg import -a %s --region %s --vm-size shared-cpu-1x postgres://postgres:x@%s.internal/test",
		appName, f.PrimaryRegion(), appName,
	)
	require.NotEqual(f, 0, result.ExitCode())
	require.Contains(f, result.StdOut().String(), "database \"test\" does not exist")

	// Wait for the importer machine to be destroyed.
	time.Sleep(5 * time.Second)

	// Even with the error, the importer machine should have been
	// destroyed.
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))
}
