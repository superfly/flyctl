//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestPostgres_singleNode(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName() // Since this explicitly sets a size, no need to test on GPUs/alternate

	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	// Retry pg create up to 3 times due to transient volume provisioning and health check timeout issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1",
			f.OrgSlug(), appName, f.PrimaryRegion(),
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(10 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	f.Fly("config validate")
}

func TestPostgres_autostart(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	// Since this explicitly sets a size, no need to test on GPUs/alternate
	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()

	// Retry pg create up to 3 times due to transient volume provisioning and health check timeout issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size %s --volume-size 1",
			f.OrgSlug(), appName, f.PrimaryRegion(), postgresMachineSize,
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(10 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

	machList := f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]

	for _, svc := range firstMachine.Config.Services {
		require.NotNil(t, svc.Autostart, "autostart when not set defaults to True")
		require.False(t, *svc.Autostart, "autostart wasn't disabled")
	}

	appName = f.CreateRandomAppName()

	// Retry second pg create
	pgCreateErr = nil
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --autostart",
			f.OrgSlug(), appName, f.PrimaryRegion(),
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(10 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

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

	// Since this explicitly sets a size, no need to test on GPUs/alternate
	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()
	findLeaderID := func(ml []*fly.Machine) string {
		for _, mach := range ml {
			for _, chk := range mach.Checks {
				if chk.Name == "role" && chk.Output == "primary" {
					return mach.ID
				}
			}
		}
		return ""
	}

	// Retry pg create up to 3 times due to transient volume provisioning and health check timeout issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --flex --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1",
			f.OrgSlug(), appName, f.PrimaryRegion(),
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(10 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

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

	// Since this explicitly sets a size, no need to test on GPUs/alternate
	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()

	// Retry pg create up to 3 times due to transient volume provisioning issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1",
			f.OrgSlug(), appName, f.PrimaryRegion(),
		)

		if result.ExitCode() == 0 {
			// Success!
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		// If this was a volume-related error and we have retries left, clean up and retry
		if strings.Contains(result.StdErrString(), "volume not found") && attempt < 3 {
			f.Logf("Volume provisioning failed (attempt %d/3), retrying...", attempt)
			// Clean up the partially created app before retrying
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(5 * time.Second) // Give the platform time to clean up
		} else if attempt < 3 {
			// Other error, still retry but don't clean up
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, result.StdErrString())
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

	machList := f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]
	f.Fly("m destroy %s -a %s --force", firstMachine.ID, appName)
	result := f.Fly("status -a %s", appName)

	require.Contains(f, result.StdOut().String(), "No machines are available on this app")
}

func TestPostgres_haConfigSave(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	// Since this explicitly sets a size, no need to test on GPUs/alternate
	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()

	// Retry pg create up to 3 times due to transient volume provisioning and health check timeout issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1",
			f.OrgSlug(), appName, f.PrimaryRegion(),
		)

		if result.ExitCode() == 0 {
			// Success!
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		// Clean up and retry on volume errors, timeouts, or "name taken" errors (indicates partial creation)
		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			// Clean up the partially created app before retrying
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(10 * time.Second) // Give the platform time to clean up
		} else if attempt < 3 {
			// Other error, still retry but don't clean up
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 3, len(ml))
	require.Equal(f, "shared-cpu-1x", ml[0].Config.Guest.ToSize())
	f.Fly("config validate")
}

const postgresMachineSize = "shared-cpu-4x"

// assertMachineCount checks the number of machines for the given app.
func assertMachineCount(tb assert.TestingT, f *testlib.FlyctlTestEnv, appName string, expected int) {
	machines := f.MachinesList(appName)

	var xs []string
	for _, m := range machines {
		xs = append(xs, fmt.Sprintf("machine %s (image: %s)", m.ID, m.FullImageRef()))
	}
	assert.Len(tb, machines, expected, "expected %d machine(s) but got %s", expected, strings.Join(xs, ", "))
}

// assertPostgresIsUp checks that the given Postgres server is really up.
// Even after "fly pg create", sometimes the server is not ready for accepting connections.
func assertPostgresIsUp(tb assert.TestingT, f *testlib.FlyctlTestEnv, appName string) {
	ssh := f.FlyAllowExitFailure(`ssh console -a %s -u postgres -C "psql -p 5433 -h /run/postgresql -c 'SELECT 1'"`, appName)
	assert.Equal(tb, 0, ssh.ExitCode(), "failed to connect to postgres at %s: %s", appName, ssh.StdErr())
}

func TestPostgres_ImportSuccess(t *testing.T) {
	t.Skip()

	f := testlib.NewTestEnvFromEnv(t)

	// Since this explicitly sets a size, no need to test on GPUs/alternate
	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	firstAppName := f.CreateRandomAppName()
	secondAppName := f.CreateRandomAppName()

	// Retry first pg create up to 3 times due to transient volume provisioning and health check timeout issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size %s --volume-size 1 --password x",
			f.OrgSlug(), firstAppName, f.PrimaryRegion(), postgresMachineSize,
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", firstAppName)
			time.Sleep(10 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")

	// Retry second pg create
	pgCreateErr = nil
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size %s --volume-size 1",
			f.OrgSlug(), secondAppName, f.PrimaryRegion(), postgresMachineSize,
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		stderr := result.StdErrString()
		needsCleanup := strings.Contains(stderr, "volume not found") ||
			strings.Contains(stderr, "context deadline exceeded") ||
			strings.Contains(stderr, "Name has already been taken")

		if needsCleanup && attempt < 3 {
			f.Logf("pg create failed (attempt %d/3), cleaning up and retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", secondAppName)
			time.Sleep(10 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertPostgresIsUp(t, f, firstAppName)
	}, 1*time.Minute, 10*time.Second)

	f.Fly(
		"ssh console -a %s -u postgres -C \"psql -p 5433 -h /run/postgresql -c 'CREATE TABLE app_name (app_name TEXT)'\"",
		firstAppName,
	)
	f.Fly(
		"ssh console -a %s -u postgres -C \"psql -p 5433 -h /run/postgresql -c \\\"INSERT INTO app_name VALUES ('%s')\\\"\"",
		firstAppName, firstAppName,
	)

	f.Fly(
		"pg import -a %s --region %s --vm-size %s postgres://postgres:x@%s.internal/postgres",
		secondAppName, f.PrimaryRegion(), postgresMachineSize, firstAppName,
	)

	result := f.Fly(
		"ssh console -a %s -u postgres -C \"psql -p 5433 -h /run/postgresql -c 'SELECT app_name FROM app_name'\"",
		secondAppName,
	)
	output := result.StdOut().String()
	require.Contains(f, output, firstAppName)

	// Wait for the importer machine to be destroyed.
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertMachineCount(t, f, secondAppName, 1)
	}, 2*time.Minute, 10*time.Second, "import machine not destroyed")
}

func TestPostgres_ImportFailure(t *testing.T) {
	t.Skip()

	f := testlib.NewTestEnvFromEnv(t)

	// Since this explicitly sets a size, no need to test on GPUs/alternate
	// sizes.
	if f.VMSize != "" {
		t.Skip()
	}

	appName := f.CreateRandomAppName()

	// Retry pg create up to 3 times due to transient volume provisioning issues
	var pgCreateErr error
	for attempt := 1; attempt <= 3; attempt++ {
		result := f.FlyAllowExitFailure(
			"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size %s --volume-size 1 --password x",
			f.OrgSlug(), appName, f.PrimaryRegion(), postgresMachineSize,
		)

		if result.ExitCode() == 0 {
			pgCreateErr = nil
			break
		}

		pgCreateErr = fmt.Errorf("pg create failed (attempt %d/3): %s", attempt, result.StdErrString())

		if strings.Contains(result.StdErrString(), "volume not found") && attempt < 3 {
			f.Logf("Volume provisioning failed (attempt %d/3), retrying...", attempt)
			f.FlyAllowExitFailure("apps destroy %s --yes", appName)
			time.Sleep(5 * time.Second)
		} else if attempt < 3 {
			f.Logf("pg create failed (attempt %d/3): %v, retrying...", attempt, result.StdErrString())
			time.Sleep(2 * time.Second)
		}
	}

	require.NoError(f, pgCreateErr, "pg create failed after 3 attempts")
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertPostgresIsUp(t, f, appName)
	}, 1*time.Minute, 10*time.Second)

	result := f.FlyAllowExitFailure(
		"pg import -a %s --region %s --vm-size %s postgres://postgres:x@%s.internal/test",
		appName, f.PrimaryRegion(), postgresMachineSize, appName,
	)
	require.NotEqual(f, 0, result.ExitCode())
	require.Contains(f, result.StdOut().String(), "database \"test\" does not exist")

	// Wait for the importer machine to be destroyed.
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertMachineCount(t, f, appName, 1)
	}, 1*time.Minute, 10*time.Second, "import machine not destroyed")
}
