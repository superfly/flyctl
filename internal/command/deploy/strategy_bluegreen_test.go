package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func TestNew(t *testing.T) {
	strategy := BlueGreenStrategy(&machineDeployment{}, nil)
	assert.False(t, strategy.isAborted())
}

func newBlueGreenStrategy(client flapsutil.FlapsClient, numberOfExistingMachines int) *blueGreen {
	var machines []*machineUpdateEntry
	ios, _, _, _ := iostreams.Test()

	// testImageRef is a stable ImageRef used by test helpers. All machines in
	// a test share the same image so DetectMultipleImageVersions passes without
	// needing to exercise the image-check logic in every test.
	testImageRef := fly.MachineImageRef{Repository: "test-app", Tag: "test"}
	for i := range numberOfExistingMachines {
		machines = append(machines, &machineUpdateEntry{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{
				ID:         fmt.Sprintf("%x", i+1),
				ImageRef:   testImageRef,
				HostStatus: fly.HostStatusOk,
			}, false),
			launchInput: &fly.LaunchMachineInput{
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
				MinSecretsVersion: nil,
			},
		})
	}
	strategy := &blueGreen{
		apiClient:       &mockWebClient{},
		flaps:           client,
		maxConcurrent:   10,
		appConfig:       &appconfig.Config{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		clearLinesAbove: func(int) {}, // no-op; avoids nil-panic in render loop
		timeout:         5 * time.Second,
		blueMachines:    machines,
		app:             &flaps.App{Name: "test-app"},
	}
	strategy.initialize()

	// Don't have to wait during tests.
	strategy.waitBeforeStop = 0
	strategy.waitBeforeCordon = 0
	strategy.uncordonRetryDelay = 0
	strategy.imageRefRetryDelay = 0

	return strategy
}

func TestDeploy(t *testing.T) {
	flapsClient := &mockFlapsClient{}

	ctx := context.Background()

	// Some functions take a client from the context.
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	// Happy cases
	t.Run("replace 1 machine", func(t *testing.T) {
		flapsClient.breakLaunch = false
		strategy := newBlueGreenStrategy(flapsClient, 1)

		err := strategy.Deploy(ctx)
		assert.NoError(t, err)
	})
	t.Run("replace 10 machine", func(t *testing.T) {
		flapsClient.breakLaunch = false
		strategy := newBlueGreenStrategy(flapsClient, 10)

		err := strategy.Deploy(ctx)
		assert.NoError(t, err)
	})

	// Error cases
	t.Run("no existing machines", func(t *testing.T) {
		strategy := newBlueGreenStrategy(flapsClient, 0)

		err := strategy.Deploy(ctx)
		assert.ErrorContains(t, err, "found multiple image versions")
	})
	t.Run("failed to launch machines", func(t *testing.T) {
		flapsClient.breakLaunch = true
		strategy := newBlueGreenStrategy(flapsClient, 1)

		err := strategy.Deploy(ctx)
		assert.ErrorContains(t, err, "failed to create green machines")
	})

	// A blue machine on a non-ok host must not block the deploy at any stage:
	// it is skipped by the image check and the cordon/stop/wait teardown, and
	// force-destroyed at the end alongside the healthy blues.
	t.Run("non-ok blue machine is skipped and destroyed", func(t *testing.T) {
		flapsClient.breakLaunch = false
		strategy := newBlueGreenStrategy(flapsClient, 2)
		unreachable := strategy.blueMachines[1].leasableMachine.Machine()
		unreachable.HostStatus = fly.HostStatusUnreachable
		unreachable.ImageRef = fly.MachineImageRef{}

		err := strategy.Deploy(ctx)
		assert.NoError(t, err)
	})
}

func TestMarkGreenMachinesAsReadyForTrafficRetries(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	// makeStrategyWithGreenMachines builds a blueGreen with pre-populated green
	// machines, letting us test MarkGreenMachinesAsReadyForTraffic in isolation
	// without running the full deploy pipeline.
	makeStrategyWithGreenMachines := func(client *mockFlapsClient, greenCount int) *blueGreen {
		bg := newBlueGreenStrategy(client, 0)
		for i := range greenCount {
			bg.greenMachines = append(bg.greenMachines, &machineUpdateEntry{
				leasableMachine: machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: fmt.Sprintf("green-%d", i+1)}, false),
				launchInput:     &fly.LaunchMachineInput{},
			})
		}

		return bg
	}

	ctx := context.Background()

	t.Run("succeeds immediately when no errors occur", func(t *testing.T) {
		client := &mockFlapsClient{}
		bg := makeStrategyWithGreenMachines(client, 3)

		err := bg.MarkGreenMachinesAsReadyForTraffic(ctx)
		assert.NoError(t, err)
	})

	t.Run("succeeds after transient uncordon failures are retried", func(t *testing.T) {
		client := &mockFlapsClient{uncordonTransientFailures: 2}
		bg := makeStrategyWithGreenMachines(client, 1)

		err := bg.MarkGreenMachinesAsReadyForTraffic(ctx)
		assert.NoError(t, err)

		client.mu.Lock()
		remaining := client.uncordonTransientFailures
		client.mu.Unlock()
		assert.Equal(t, 0, remaining, "all transient failures should have been consumed by retries")
	})

	t.Run("fails after all retry attempts are exhausted", func(t *testing.T) {
		client := &mockFlapsClient{breakUncordon: true}
		bg := makeStrategyWithGreenMachines(client, 1)
		bg.uncordonRetryAttempts = 3

		err := bg.MarkGreenMachinesAsReadyForTraffic(ctx)
		assert.ErrorContains(t, err, "failed to uncordon")
	})
}

func FuzzDeploy(f *testing.F) {
	flapsClient := &mockFlapsClient{}

	ctx := context.Background()

	// Some functions take a client from the context.
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	f.Add(20, false, false, false, false)

	f.Fuzz(func(t *testing.T, numberOfExistingMachines int, breakLaunch bool, breakWait bool, breakUncordon bool, breakSetMetadata bool) {
		strategy := newBlueGreenStrategy(flapsClient, numberOfExistingMachines)
		flapsClient.breakLaunch = breakLaunch
		flapsClient.breakWait = breakWait
		flapsClient.breakUncordon = breakUncordon
		flapsClient.breakSetMetadata = breakSetMetadata

		// At least, Deploy must not panic.
		strategy.Deploy(ctx)
	})
}

// ---------------------------------------------------------------------------
// Tests for the SkipLaunch / health-check fixes
// ---------------------------------------------------------------------------

// TestMachineHasConfiguredChecks verifies the helper that decides whether a
// machine config carries any health-check definitions.
func TestMachineHasConfiguredChecks(t *testing.T) {
	t.Run("no checks at all", func(t *testing.T) {
		cfg := &fly.MachineConfig{}
		assert.False(t, machineHasConfiguredChecks(cfg))
	})

	t.Run("top-level check", func(t *testing.T) {
		cfg := &fly.MachineConfig{
			Checks: map[string]fly.MachineCheck{"alive": {}},
		}
		assert.True(t, machineHasConfiguredChecks(cfg))
	})

	t.Run("service-level check only", func(t *testing.T) {
		cfg := &fly.MachineConfig{
			Services: []fly.MachineService{
				{Checks: []fly.MachineServiceCheck{{}}},
			},
		}
		assert.True(t, machineHasConfiguredChecks(cfg))
	})

	t.Run("service with no checks", func(t *testing.T) {
		cfg := &fly.MachineConfig{
			Services: []fly.MachineService{
				{Checks: nil},
			},
		}
		assert.False(t, machineHasConfiguredChecks(cfg))
	})
}

// newBlueGreenStrategyWithState is like newBlueGreenStrategy but lets the
// caller specify the state of each blue machine and its SkipLaunch value.
// This is used to simulate machines that have been auto-stopped.
func newBlueGreenStrategyWithState(client flapsutil.FlapsClient, machineState string, skipLaunch bool) *blueGreen {
	ios, _, _, _ := iostreams.Test()

	testImageRef := fly.MachineImageRef{Repository: "test-app", Tag: "test"}
	machines := []*machineUpdateEntry{
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{
				State:      machineState,
				ImageRef:   testImageRef,
				HostStatus: fly.HostStatusOk,
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
			}, false),
			launchInput: &fly.LaunchMachineInput{
				SkipLaunch: skipLaunch,
				Config: &fly.MachineConfig{
					Metadata: map[string]string{},
					Checks: map[string]fly.MachineCheck{
						"check1": {},
					},
				},
				MinSecretsVersion: nil,
			},
		},
	}

	strategy := &blueGreen{
		apiClient:       &mockWebClient{},
		flaps:           client,
		maxConcurrent:   10,
		appConfig:       &appconfig.Config{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		clearLinesAbove: func(int) {},
		timeout:         5 * time.Second,
		blueMachines:    machines,
		app:             &flaps.App{Name: "test-app"},
	}
	strategy.initialize()
	strategy.waitBeforeStop = 0
	strategy.waitBeforeCordon = 0
	strategy.uncordonRetryDelay = 0
	strategy.imageRefRetryDelay = 0

	return strategy
}

// TestCreateGreenMachinesForceStartsRepresentative verifies that when the
// only blue machine in a process group with configured health checks would
// stay stopped (SkipLaunch=true), its green replacement is promoted to
// SkipLaunch=false so that at least one machine gets health-verified.
func TestCreateGreenMachinesForceStartsRepresentative(t *testing.T) {
	client := &mockFlapsClient{}
	ctx := context.Background()
	ctx = flapsutil.NewContextWithClient(ctx, client)

	// Simulate a stopped blue machine: SkipLaunch=true. It has configured
	// checks, so its green replacement must be force-started as the group's
	// health-check representative.
	strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)

	err := strategy.CreateGreenMachines(ctx)
	assert.NoError(t, err)
	assert.Len(t, strategy.greenMachines, 1, "expected one green machine to be created")

	client.mu.Lock()
	inputs := client.launchInputs
	client.mu.Unlock()

	assert.Len(t, inputs, 1, "expected one Launch call")
	assert.False(t, inputs[0].SkipLaunch,
		"green machine must be launched with SkipLaunch=false as the process group's health-check representative")
}

// makeMachineEntry constructs a machineUpdateEntry with just enough state to
// exercise forceStartRepresentatives: a process-group tag, a SkipLaunch value,
// and optional configured checks.
func makeMachineEntry(processGroup string, skipLaunch, hasChecks bool) *machineUpdateEntry {
	checks := map[string]fly.MachineCheck{}
	if hasChecks {
		checks["check1"] = fly.MachineCheck{}
	}

	return &machineUpdateEntry{
		launchInput: &fly.LaunchMachineInput{
			SkipLaunch: skipLaunch,
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyProcessGroup: processGroup,
				},
				Checks: checks,
			},
		},
	}
}

// TestForceStartRepresentatives exercises the per-process-group selection
// logic that decides which green machines must be launched even when their
// blue counterparts were stopped.
func TestForceStartRepresentatives(t *testing.T) {
	t.Run("stopped group with checks forces the first machine", func(t *testing.T) {
		bg := &blueGreen{blueMachines: []*machineUpdateEntry{
			makeMachineEntry("app", true, true),
		}}

		assert.Equal(t, map[int]bool{0: true}, bg.forceStartRepresentatives())
	})

	t.Run("stopped group without checks is left alone", func(t *testing.T) {
		bg := &blueGreen{blueMachines: []*machineUpdateEntry{
			makeMachineEntry("worker", true, false),
		}}

		assert.Empty(t, bg.forceStartRepresentatives(),
			"no checks means no representative needed; green should mirror blue's stopped state")
	})

	t.Run("group with a natural starter needs no forcing", func(t *testing.T) {
		bg := &blueGreen{blueMachines: []*machineUpdateEntry{
			makeMachineEntry("app", true, true),  // stopped
			makeMachineEntry("app", false, true), // already going to start
		}}

		assert.Empty(t, bg.forceStartRepresentatives(),
			"a machine that already starts satisfies the invariant")
	})

	t.Run("multiple stopped in same group forces exactly one", func(t *testing.T) {
		bg := &blueGreen{blueMachines: []*machineUpdateEntry{
			makeMachineEntry("app", true, true),
			makeMachineEntry("app", true, true),
			makeMachineEntry("app", true, true),
		}}

		assert.Equal(t, map[int]bool{0: true}, bg.forceStartRepresentatives(),
			"only the first stopped machine in the group should be promoted")
	})

	t.Run("multiple stopped groups get one representative each", func(t *testing.T) {
		bg := &blueGreen{blueMachines: []*machineUpdateEntry{
			makeMachineEntry("app", true, true),
			makeMachineEntry("worker", true, true),
			makeMachineEntry("app", true, true),
			makeMachineEntry("worker", true, true),
		}}

		assert.Equal(t, map[int]bool{0: true, 1: true}, bg.forceStartRepresentatives(),
			"each process group with checks needs its own representative")
	})

	t.Run("only groups with checks get a representative", func(t *testing.T) {
		bg := &blueGreen{blueMachines: []*machineUpdateEntry{
			makeMachineEntry("app", true, true),     // has checks -> force
			makeMachineEntry("worker", true, false), // no checks -> mirror blue
		}}

		assert.Equal(t, map[int]bool{0: true}, bg.forceStartRepresentatives())
	})
}

// TestCreateGreenMachinesMirrorsBlueSkipLaunchForNonRepresentatives verifies
// that, given multiple stopped blue machines in a process group with checks,
// exactly one green is force-started and the rest inherit SkipLaunch=true
// (so they mirror the pre-deploy stopped state).
func TestCreateGreenMachinesMirrorsBlueSkipLaunchForNonRepresentatives(t *testing.T) {
	client := &mockFlapsClient{}
	ctx := context.Background()
	ctx = flapsutil.NewContextWithClient(ctx, client)

	ios, _, _, _ := iostreams.Test()
	blues := []*machineUpdateEntry{
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{}, false),
			launchInput:     makeMachineEntry("app", true, true).launchInput,
		},
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{}, false),
			launchInput:     makeMachineEntry("app", true, true).launchInput,
		},
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{}, false),
			launchInput:     makeMachineEntry("app", true, true).launchInput,
		},
	}

	strategy := &blueGreen{
		apiClient:       &mockWebClient{},
		flaps:           client,
		maxConcurrent:   10,
		appConfig:       &appconfig.Config{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		clearLinesAbove: func(int) {},
		timeout:         5 * time.Second,
		blueMachines:    blues,
		app:             &flaps.App{Name: "test-app"},
	}
	strategy.initialize()
	strategy.waitBeforeStop = 0
	strategy.waitBeforeCordon = 0
	strategy.uncordonRetryDelay = 0

	err := strategy.CreateGreenMachines(ctx)
	assert.NoError(t, err)
	assert.Len(t, strategy.greenMachines, 3)

	client.mu.Lock()
	inputs := client.launchInputs
	client.mu.Unlock()

	require.Len(t, inputs, 3)
	started := 0
	for _, in := range inputs {
		if !in.SkipLaunch {
			started++
		}
	}
	assert.Equal(t, 1, started,
		"exactly one green machine in the group must be force-started; the rest mirror blue's stopped state")
}

// TestDeployWithStoppedBlueMachinesEnforcesHealthChecks verifies the full
// deploy pipeline when blue machines have SkipLaunch=true (auto-stopped).
//
// Before the fix, the deploy would silently succeed: green machines were
// never started and their health was faked as "1/1 passing".
//
// After the fix, the deploy must attempt real health checks and only succeed
// when they pass, or fail/roll back when they don't.
func TestDeployWithStoppedBlueMachinesEnforcesHealthChecks(t *testing.T) {
	t.Run("fails when health checks cannot be verified", func(t *testing.T) {
		// breakGet=true simulates the platform being unreachable for health polls.
		client := &mockFlapsClient{breakGet: true}
		ctx := context.Background()
		ctx = flapsutil.NewContextWithClient(ctx, client)

		strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)
		// Short timeout so the test doesn't hang.
		strategy.timeout = 500 * time.Millisecond

		err := strategy.Deploy(ctx)
		assert.Error(t, err,
			"deploy must fail when health checks cannot be verified, not silently succeed")
	})

	t.Run("succeeds when health checks pass", func(t *testing.T) {
		// Default mockFlapsClient.Get returns a passing machine.
		client := &mockFlapsClient{}
		ctx := context.Background()
		ctx = flapsutil.NewContextWithClient(ctx, client)

		strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)

		err := strategy.Deploy(ctx)
		assert.NoError(t, err, "deploy must succeed when health checks pass")
	})
}

// TestBlueGreenAbortsWhenGreenChecksFailAfterConfigChange exercises the
// primary value of the fix: catching an app-level misconfiguration that
// silently breaks health checks on the new deploy.
//
// Concrete real-world regressions this protects against:
//
//   - The new config changes `internal_port` (or the app now listens on a
//     different port), so the health probe can't reach the process at all.
//   - The app adds `force_ssl` (e.g. Phoenix's default) or another redirect
//     rule that traps the health probe with a 3xx, so `/` never returns 200.
//   - Any other startup-time regression that keeps the machine "running" but
//     makes the configured HTTP check fail.
//
// Pre-fix behaviour with auto-stopped blue machines: green machines were
// silently marked "1/1 passing" without ever running the check, blues were
// destroyed, and traffic hit a broken app. The representative-force-start
// fix ensures at least one green in each check-having group actually starts
// and gets polled — which is exactly what surfaces these regressions.
func TestBlueGreenAbortsWhenGreenChecksFailAfterConfigChange(t *testing.T) {
	// unhealthyGet=true makes the mock return a Machine whose one configured
	// check reports Critical, mimicking any of the misconfiguration modes
	// listed above from the poller's point of view.
	client := &mockFlapsClient{unhealthyGet: true}
	ctx := context.Background()
	ctx = flapsutil.NewContextWithClient(ctx, client)

	// Every blue is stopped (auto_stop_machines scenario). Without the fix,
	// all greens would inherit SkipLaunch=true and skip health polling
	// entirely — the misconfiguration would slip through undetected.
	strategy := newBlueGreenStrategyWithState(client, fly.MachineStateStopped, true)
	strategy.timeout = 500 * time.Millisecond // don't let the test hang

	err := strategy.Deploy(ctx)
	assert.Error(t, err,
		"deploy must abort when the representative green machine's health checks fail, "+
			"even though blue machines were all auto-stopped "+
			"(config regressions like a bad internal_port or force_ssl redirect must be caught)")
}

// ---------------------------------------------------------------------------
// Tests for DetectMultipleImageVersions / image-ref lookup robustness
// ---------------------------------------------------------------------------

// newStrategyWithImages builds a blueGreen whose blue machines each carry a
// specific ImageRef so DetectMultipleImageVersions can be exercised without
// reaching the rest of the deploy pipeline.
func newStrategyWithImages(client flapsutil.FlapsClient, images ...fly.MachineImageRef) *blueGreen {
	ios, _, _, _ := iostreams.Test()
	var machines []*machineUpdateEntry
	for _, img := range images {
		machines = append(machines, &machineUpdateEntry{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{
				ID: func() string {
					if img.Tag != "" {
						return "m-" + img.Tag
					}

					return fmt.Sprintf("m-unreachable-%d", len(machines))
				}(),
				ImageRef:   img,
				HostStatus: fly.HostStatusOk,
				Config:     &fly.MachineConfig{Metadata: map[string]string{}},
			}, false),
			launchInput: &fly.LaunchMachineInput{
				Config: &fly.MachineConfig{Metadata: map[string]string{}},
			},
		})
	}
	strategy := &blueGreen{
		apiClient:     &mockWebClient{},
		flaps:         client,
		maxConcurrent: 10,
		appConfig:     &appconfig.Config{AppName: "test-app"},
		io:            ios,
		colorize:      ios.ColorScheme(),
		timeout:       5 * time.Second,
		blueMachines:  machines,
		app:           &flaps.App{Name: "test-app"},
	}
	strategy.initialize()
	strategy.imageRefRetryDelay = 0

	return strategy
}

// TestDetectMultipleImageVersions_SingleImage verifies the happy path:
// all machines on the same image passes the check.
func TestDetectMultipleImageVersions_SingleImage(t *testing.T) {
	client := &mockFlapsClient{}
	ctx := context.Background()

	sameImage := fly.MachineImageRef{Repository: "registry.fly.io/myapp", Tag: "deployment-01"}
	strategy := newStrategyWithImages(client, sameImage, sameImage, sameImage)

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.NoError(t, err)
}

// TestDetectMultipleImageVersions_DifferentImages verifies that genuinely
// different image versions across blue machines are still caught.
func TestDetectMultipleImageVersions_DifferentImages(t *testing.T) {
	client := &mockFlapsClient{}
	ctx := context.Background()

	imgA := fly.MachineImageRef{Repository: "registry.fly.io/myapp", Tag: "deployment-01"}
	imgB := fly.MachineImageRef{Repository: "registry.fly.io/myapp", Tag: "deployment-02"}
	strategy := newStrategyWithImages(client, imgA, imgA, imgB)

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.ErrorIs(t, err, ErrMultipleImageVersions)
}

// TestDetectMultipleImageVersions_EmptyImageRefRefreshSucceeds verifies that
// when one machine's ImageRef comes back empty from the list API, a fresh Get
// returning real image data allows the check to proceed.
func TestDetectMultipleImageVersions_EmptyImageRefRefreshSucceeds(t *testing.T) {
	realImage := fly.MachineImageRef{Repository: "registry.fly.io/myapp", Tag: "deployment-01"}

	// The mock Get returns a machine on a healthy host with a real ImageRef.
	client := &mockFlapsClient{
		GetFunc: func(_ context.Context, _ string, machineID string) (*fly.Machine, error) {
			return &fly.Machine{ID: machineID, ImageRef: realImage, HostStatus: fly.HostStatusOk}, nil
		},
	}
	ctx := context.Background()

	// One machine has the correct image; one has an empty ImageRef (simulates
	// the list API returning incomplete data for an unreachable host).
	strategy := newStrategyWithImages(client,
		realImage,
		fly.MachineImageRef{}, // empty — should be refreshed via Get
	)

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.NoError(t, err, "deploy should succeed when the refreshed machine carries the same image")
}

// TestDetectMultipleImageVersions_UnreachableHostStatus_Proceeds verifies the
// primary detection path: the list API itself reports host_status=unreachable.
// The machine must be excluded from the image tally without any Get lookups,
// and the deploy proceeds (the machine gets replaced on a healthy host).
func TestDetectMultipleImageVersions_UnreachableHostStatus_Proceeds(t *testing.T) {
	realImage := fly.MachineImageRef{Repository: "registry.fly.io/test-app", Tag: "deployment-01"}

	// breakGet guards the "no Get lookups" claim: if the code wrongly fell
	// back to a Get here, the refresh loop would burn retries; the outcome
	// stays the same but the explicit HostStatus must short-circuit first.
	client := &mockFlapsClient{breakGet: true}
	ctx := context.Background()

	strategy := newStrategyWithImages(client, realImage, realImage, fly.MachineImageRef{})
	strategy.blueMachines[2].leasableMachine.Machine().HostStatus = fly.HostStatusUnreachable

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.NoError(t, err,
		"a machine on an unreachable host must not block the deploy — it gets replaced")
}

// TestDetectMultipleImageVersions_UnknownHostStatus_Proceeds verifies that any
// host status other than "ok" — not just "unreachable" — excludes the machine
// from image verification: an unknown host can't be trusted to respond either.
func TestDetectMultipleImageVersions_UnknownHostStatus_Proceeds(t *testing.T) {
	realImage := fly.MachineImageRef{Repository: "registry.fly.io/test-app", Tag: "deployment-01"}

	client := &mockFlapsClient{breakGet: true}
	ctx := context.Background()

	strategy := newStrategyWithImages(client, realImage, realImage, fly.MachineImageRef{})
	strategy.blueMachines[2].leasableMachine.Machine().HostStatus = fly.HostStatusUnknown

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.NoError(t, err,
		"a machine on an unknown host must not block the deploy — it gets replaced")
}

// TestDetectMultipleImageVersions_UnreachableHost_Proceeds verifies the
// reported production scenario: 36 ok machines + 1 whose refreshed data comes
// back with host_status=unreachable. The deploy must proceed (replacing the
// unreachable machine), not fail with a misleading "different images" error.
func TestDetectMultipleImageVersions_UnreachableHost_Proceeds(t *testing.T) {
	realImage := fly.MachineImageRef{Repository: "registry.fly.io/test-app", Tag: "deployment-01"}

	client := &mockFlapsClient{
		GetFunc: func(_ context.Context, _ string, machineID string) (*fly.Machine, error) {
			return &fly.Machine{
				ID:         machineID,
				HostStatus: fly.HostStatusUnreachable,
				ImageRef:   fly.MachineImageRef{},
			}, nil
		},
	}
	ctx := context.Background()

	images := make([]fly.MachineImageRef, 36)
	for i := range images {
		images[i] = realImage
	}
	images = append(images, fly.MachineImageRef{}) // 1 unreachable
	strategy := newStrategyWithImages(client, images...)

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.NoError(t, err,
		"unreachable machines must not block the deploy — they get replaced")
}

// TestDetectMultipleImageVersions_RealConflictStillBlocks ensures that
// skipping unreachable machines does not bypass a genuine image-version
// conflict among the reachable machines.
func TestDetectMultipleImageVersions_RealConflictStillBlocks(t *testing.T) {
	imgA := fly.MachineImageRef{Repository: "registry.fly.io/test-app", Tag: "deployment-01"}
	imgB := fly.MachineImageRef{Repository: "registry.fly.io/test-app", Tag: "deployment-02"}

	// Get returns the unreachable machine with empty ImageRef.
	client := &mockFlapsClient{
		GetFunc: func(_ context.Context, _ string, machineID string) (*fly.Machine, error) {
			return &fly.Machine{ID: machineID, HostStatus: fly.HostStatusUnreachable}, nil
		},
	}
	ctx := context.Background()

	// Two machines with different real images + 1 unreachable.
	strategy := newStrategyWithImages(client, imgA, imgB, fly.MachineImageRef{})

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.ErrorIs(t, err, ErrMultipleImageVersions,
		"unreachable machines must not mask a genuine image-version conflict")
}

// TestDetectMultipleImageVersions_GetError_Proceeds verifies that a hard Get
// failure (API error) after all retries is treated like an unreachable host:
// the machine is skipped and replaced, not reported as a version conflict.
func TestDetectMultipleImageVersions_GetError_Proceeds(t *testing.T) {
	realImage := fly.MachineImageRef{Repository: "registry.fly.io/test-app", Tag: "deployment-01"}
	client := &mockFlapsClient{breakGet: true}
	ctx := context.Background()

	strategy := newStrategyWithImages(client,
		realImage,
		fly.MachineImageRef{},
	)
	strategy.imageRefRetryAttempts = 1

	err := strategy.DetectMultipleImageVersions(ctx)
	assert.NoError(t, err,
		"a machine whose image cannot be verified must be replaced, not block the deploy")
}

// TestFormatDestroyCommand verifies the destroy-command formatter produces
// copy-paste-ready output for both single and multi-machine cases.
func TestFormatDestroyCommand(t *testing.T) {
	t.Run("single machine", func(t *testing.T) {
		cmd := formatDestroyCommand("my-app", []string{"abc123"})
		assert.Equal(t, "fly machine destroy --force -a my-app abc123", cmd)
	})

	t.Run("multiple machines uses backslash continuation", func(t *testing.T) {
		cmd := formatDestroyCommand("my-app", []string{"aaa111", "bbb222", "ccc333"})
		assert.Contains(t, cmd, "fly machine destroy --force -a my-app")
		assert.Contains(t, cmd, "aaa111")
		assert.Contains(t, cmd, "bbb222")
		assert.Contains(t, cmd, "ccc333")
		// Must have backslash continuations so each ID is on its own line.
		assert.Contains(t, cmd, " \\\n", "expected backslash continuation for multi-machine command")
	})
}
