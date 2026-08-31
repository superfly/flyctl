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

// flyctlLaunchIDMetadataKey mirrors machine.FlyctlLaunchIDMetadataKey from
// internal/machine/launch.go. It's redeclared here so this integration test
// doesn't depend on the internal package.
const flyctlLaunchIDMetadataKey = "fly_flyctl_launch_id"

func TestFlyDeployBluegreenImplicitAppProcessGroup(t *testing.T) {
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

// TestFlyDeploy_BlueGreen_LaunchIdempotencyMetadata is an end-to-end regression
// test for the resilience improvements to the bluegreen strategy. It exists
// primarily to guard the two changes that prevent transient flaps failures
// from stranding a deployment with both blue and green machines live:
//
//  1. Every green machine created during a bluegreen deploy must carry a
//     unique per-launch idempotency tag in its config metadata. Without it,
//     the client-side retry loop cannot detect a silent-success Launch (a
//     flaps 408 that actually committed the machine) and could produce
//     duplicate green machines on retry.
//  2. A successful bluegreen deploy must still complete end-to-end and leave
//     only one image version running. This guards against regressions in the
//     retry / checkpoint plumbing where a well-intentioned change might
//     silently break the happy path.
//
// The failure modes fixed by the retry logic (transient 408s from flaps)
// can't be reliably reproduced in preflight without infrastructure-level
// fault injection, so this test asserts the observable invariants that hold
// on any successful deploy.
func TestFlyDeploy_BlueGreen_LaunchIdempotencyMetadata(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion())

	// Bluegreen requires at least one configured healthcheck; without it the
	// strategy warns and skips health polling. Add one before the first
	// deploy so subsequent bluegreen runs behave the same way real apps do.
	appConfig := f.ReadFile("fly.toml")
	appConfig += `
  [[http_service.checks]]
    grace_period = "5s"
    interval = "10s"
    method = "GET"
    timeout = "5s"
    path = "/"
`
	f.WriteFlyToml("%s", appConfig)
	f.Fly("deploy --remote-only")

	// First bluegreen deploy — stamps a launch-id on every green machine.
	f.Fly("deploy --remote-only --strategy bluegreen")

	firstLaunchIDs := collectLaunchIDs(t, f, appName)
	require.NotEmpty(t, firstLaunchIDs,
		"every machine created by a bluegreen deploy must carry a %q metadata tag; "+
			"without it the client-side retry loop can't dedupe silent-success 408s",
		flyctlLaunchIDMetadataKey)

	for _, m := range f.MachinesList(appName) {
		assert.Equal(t, fly.MachineStateStarted, m.State,
			"machine %s must be started after a successful bluegreen deploy, got %q",
			m.ID, m.State)
		assert.NotEmpty(t, m.Config.Metadata[flyctlLaunchIDMetadataKey],
			"machine %s should carry the launch-id metadata tag", m.ID)
	}

	// Second bluegreen deploy — fresh green machines, fresh launch-ids. This
	// exercises the full retry-capable path a second time and proves the
	// checkpoint step (safe-for-deletion tagging) is idempotent-friendly:
	// a repeated deploy must still succeed.
	f.Fly("deploy --remote-only --strategy bluegreen")

	secondLaunchIDs := collectLaunchIDs(t, f, appName)
	require.NotEmpty(t, secondLaunchIDs, "second bluegreen deploy must also stamp launch-ids on machines")

	// Every launch-id from the second deploy must be different from the ones
	// on the first — they're generated fresh per machine per deploy.
	for id := range secondLaunchIDs {
		assert.NotContains(t, firstLaunchIDs, id,
			"launch-ids must be regenerated on each deploy; %s was reused across runs", id)
	}

	// After a healthy bluegreen deploy the machine count and image version
	// must both be stable — exactly what protecting against the "two versions
	// live" incident looks like from the outside.
	machines := f.MachinesList(appName)
	require.NotEmpty(t, machines, "app must have machines after bluegreen deploy")
	imageRefs := map[string]struct{}{}
	for _, m := range machines {
		assert.Equal(t, fly.MachineStateStarted, m.State,
			"machine %s must remain started after the second bluegreen deploy", m.ID)
		imageRefs[m.ImageRef.Repository+":"+m.ImageRef.Tag] = struct{}{}
	}
	assert.Len(t, imageRefs, 1,
		"a completed bluegreen deploy must leave exactly one image version running, saw %d: %v",
		len(imageRefs), imageRefs)
}

// collectLaunchIDs returns the set of unique launch-id metadata values found
// on the app's machines. An empty result signals the bluegreen strategy is
// no longer stamping the tag — which would break the idempotency retry loop
// in machine.LaunchWithIdempotency.
func collectLaunchIDs(t *testing.T, f *testlib.FlyctlTestEnv, appName string) map[string]struct{} {
	t.Helper()

	ids := map[string]struct{}{}
	for _, m := range f.MachinesList(appName) {
		if m.Config == nil {
			continue
		}
		if id := m.Config.Metadata[flyctlLaunchIDMetadataKey]; id != "" {
			ids[id] = struct{}{}
		}
	}

	return ids
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
