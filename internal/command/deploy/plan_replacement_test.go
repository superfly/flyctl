package deploy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRecoveryRetainsSuccessfulReplacement(t *testing.T) {
	for _, tc := range []struct {
		name, originalState                                     string
		canary, readinessFailure, leaseFailure, missingAccepted bool
	}{
		{name: "partial-success", originalState: "started"},
		{name: "create-lease-failure", originalState: "started", leaseFailure: true},
		{name: "stopped", originalState: "stopped"},
		{name: "canary", originalState: "started", canary: true},
		{name: "missing canary", originalState: "started", canary: true, missingAccepted: true},
		{name: "readiness-failure", originalState: "started", readinessFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withQuietIOStreams(context.Background())
			oldState := &AppState{}
			desired := &AppState{}
			for _, suffix := range []string{"-a", "-b"} {
				id := t.Name() + suffix
				t.Cleanup(func() { healthChecksPassed.Delete(id) })
				oldState.Machines = append(oldState.Machines, &fly.Machine{ID: id, State: tc.originalState, HostStatus: fly.HostStatusOk, Config: &fly.MachineConfig{Image: "old"}})
				desired.Machines = append(desired.Machines, &fly.Machine{ID: id, State: "replacing", HostStatus: fly.HostStatusOk, Config: &fly.MachineConfig{Image: "new", Mounts: []fly.MachineMount{{Name: "data", Path: "/data"}}}})
			}
			var mu sync.Mutex
			live := map[string]*fly.Machine{}
			for _, m := range oldState.Machines {
				clone := *m
				live[m.ID] = &clone
			}
			launches := 0
			destroyed := map[string]int{}
			leaseAcquisitions := map[string]int{}
			leases := map[string]string{}
			replacementReleases := map[string]int{}
			retrying := false
			canaryOmissions := 0
			expiredReleases := 0
			waitCalls := 0
			client := &mock.FlapsClient{
				AcquireLeaseFunc: func(_ context.Context, _, id string, _ *int) (*fly.MachineLease, error) {
					mu.Lock()
					leaseAcquisitions[id]++
					if tc.leaseFailure && id == "accepted-3" && leaseAcquisitions[id] == 1 {
						mu.Unlock()
						return nil, fmt.Errorf("lease temporarily unavailable")
					}
					if leases[id] != "" {
						t.Errorf("acquiring already held lease for %s", id)
					}
					leases[id] = "lease-" + id
					mu.Unlock()

					return &fly.MachineLease{Data: &fly.MachineLeaseData{Nonce: "lease-" + id}}, nil
				},
				ReleaseLeaseFunc: func(_ context.Context, _, id, nonce string) error {
					mu.Lock()
					defer mu.Unlock()
					if live[id] != nil {
						if leases[id] != nonce {
							t.Errorf("release %s used nonce %q, held %q", id, nonce, leases[id])
						}
						if id == "accepted-1" {
							replacementReleases[id]++
						}
					}
					delete(leases, id)
					if tc.name == "partial-success" && id == "accepted-3" {
						expiredReleases++
						return &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: fmt.Errorf("lease not found")}
					}

					return nil
				},
				RefreshLeaseFunc: func(_ context.Context, _, id string, _ *int, nonce string) (*fly.MachineLease, error) {
					return &fly.MachineLease{Status: "success", Data: &fly.MachineLeaseData{Nonce: nonce}}, nil
				},
				DestroyFunc: func(_ context.Context, _ string, in fly.RemoveMachineInput, _ string) error {
					mu.Lock()
					defer mu.Unlock()
					destroyed[in.ID]++
					delete(live, in.ID)
					delete(leases, in.ID)

					return nil
				},
				LaunchFunc: func(_ context.Context, _ string, in fly.LaunchMachineInput) (*fly.Machine, error) {
					mu.Lock()
					defer mu.Unlock()
					launches++
					if launches == 2 && !tc.readinessFailure {
						return nil, fmt.Errorf("launch returned 408")
					}
					if launches > 3 {
						return nil, fmt.Errorf("unexpected duplicate launch")
					}
					if in.SkipLaunch != (tc.originalState == "stopped") {
						t.Errorf("SkipLaunch = %v for original %s Machine", in.SkipLaunch, tc.originalState)
					}
					cfg := *in.Config
					cfg.Mounts = []fly.MachineMount{{Name: "data", Path: "/data", Volume: fmt.Sprintf("vol-%d", launches)}}
					m := &fly.Machine{ID: fmt.Sprintf("accepted-%d", launches), State: tc.originalState, HostStatus: fly.HostStatusOk, Config: &cfg}
					if in.LeaseTTL > 0 {
						m.LeaseNonce = "launch-" + m.ID
						leases[m.ID] = m.LeaseNonce
					}
					live[m.ID] = m

					return m, nil
				},
				ListFunc: func(context.Context, string, string) ([]*fly.Machine, error) {
					mu.Lock()
					defer mu.Unlock()
					retrying = true
					var result []*fly.Machine
					for _, m := range live {
						if tc.canary && canaryOmissions < 2 && m.ID == "accepted-1" {
							canaryOmissions++
							continue
						}
						clone := *m
						clone.LeaseNonce = ""
						result = append(result, &clone)
					}

					return result, nil
				},
				WaitFunc: func(_ context.Context, _, _ string, _ ...flaps.WaitOption) error {
					mu.Lock()
					defer mu.Unlock()
					waitCalls++
					if tc.readinessFailure && !retrying {
						return &flaps.FlapsError{ResponseStatusCode: 404, OriginalError: fmt.Errorf("readiness unavailable")}
					}

					return nil
				},
				StartFunc: func(context.Context, string, string, string) (*fly.MachineStartResponse, error) {
					t.Error("recovery must not explicitly start a Machine")
					return &fly.MachineStartResponse{}, nil
				},
				GetProcessesFunc: func(context.Context, string, string) (fly.MachinePsResponse, error) {
					return fly.MachinePsResponse{}, nil
				},
				GetFunc: func(_ context.Context, _, id string) (*fly.Machine, error) {
					mu.Lock()
					defer mu.Unlock()
					if tc.missingAccepted && canaryOmissions > 0 && id == "accepted-1" {
						return nil, fmt.Errorf("missing accepted canary")
					}
					m, ok := live[id]
					if !ok {
						return nil, fmt.Errorf("missing %s", id)
					}
					clone := *m

					return &clone, nil
				},
			}
			ctx = flapsutil.NewContextWithClient(ctx, client)
			md := &machineDeployment{flapsClient: client, io: iostreams.FromContext(ctx), app: &flaps.App{Name: "test"}, appConfig: &appconfig.Config{AppName: "test", Mounts: []appconfig.Mount{{Source: "data", Destination: "/data"}}}, img: "new", maxUnavailable: 1, deployRetries: 1, waitTimeout: time.Second, leaseTimeout: time.Minute, leaseDelayBetween: time.Hour}
			settings := updateMachineSettings{pushForward: true, skipHealthChecks: !tc.readinessFailure, skipSmokeChecks: true}
			current := oldState
			if tc.canary {
				require.NoError(t, md.updateMachinesWRecovery(ctx, &AppState{Machines: oldState.Machines[:1]}, oldState, &AppState{Machines: desired.Machines[:1]}, nil, settings))
				var err error
				current, err = md.appState(ctx, oldState)
				require.NoError(t, err)
			}
			err := md.updateMachinesWRecovery(ctx, oldState, current, desired, nil, settings)
			if tc.missingAccepted {
				require.ErrorContains(t, err, "missing accepted canary")
				require.Equal(t, 1, launches)
				require.Zero(t, destroyed["accepted-1"])
				return
			}
			require.NoError(t, err)
			if tc.canary {
				require.Equal(t, 2, canaryOmissions)
			}
			if tc.name == "partial-success" {
				require.Equal(t, 1, expiredReleases)
			}
			expectedLaunches := 3
			if tc.readinessFailure {
				expectedLaunches = 2
				require.Greater(t, waitCalls, 3, "readiness must run again on the accepted Machine")
			}
			if tc.leaseFailure {
				require.Equal(t, 2, leaseAcquisitions["accepted-3"], "retry the failed lease on the same accepted Machine")

			}
			require.Equal(t, expectedLaunches, launches, "never duplicate an accepted replacement")
			require.Positive(t, leaseAcquisitions["accepted-1"], "recovery acquires a fresh lease on the replacement")
			require.Equal(t, 1+leaseAcquisitions["accepted-1"], replacementReleases["accepted-1"], "release both launch and recovery leases")
			require.Empty(t, leases, "release every lease on live Machines, including successful creates")
			require.Len(t, live, 2, "retry must not duplicate a successful replacement")
			require.Zero(t, destroyed["accepted-1"], "retry must not destroy an accepted replacement")
			require.Equal(t, t.Name()+"-a", oldState.Machines[0].ID)
			require.Equal(t, tc.originalState, oldState.Machines[0].State)
		})
	}
}
