package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/iostreams"
)

type updateOutcomeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateOutcomeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestIsPreservedStoppedUpdate(t *testing.T) {
	preserved := &fly.MachineEvent{Type: "update", Status: fly.MachineStateStopped, Source: "flyd"}

	tests := []struct {
		name    string
		machine *fly.Machine
		version string
		want    bool
	}{
		{
			name: "matching current update event",
			machine: &fly.Machine{
				State:   fly.MachineStateStopped,
				Version: "new-version",
				Config:  &fly.MachineConfig{},
				Events:  []*fly.MachineEvent{preserved},
			},
			version: "new-version",
			want:    true,
		},
		{
			name:    "missing machine declines",
			version: "new-version",
		},
		{
			name: "missing config declines",
			machine: &fly.Machine{
				State:   fly.MachineStateStopped,
				Version: "new-version",
				Events:  []*fly.MachineEvent{preserved},
			},
			version: "new-version",
		},
		{
			name: "scheduled update declines",
			machine: &fly.Machine{
				State:   fly.MachineStateStopped,
				Version: "new-version",
				Config:  &fly.MachineConfig{Schedule: "daily"},
				Events:  []*fly.MachineEvent{preserved},
			},
			version: "new-version",
		},
		{
			name: "newer lifecycle event declines",
			machine: &fly.Machine{
				State:   fly.MachineStateStopped,
				Version: "new-version",
				Config:  &fly.MachineConfig{},
				Events: []*fly.MachineEvent{
					{Type: "exit", Status: fly.MachineStateStopped, Source: "flyd"},
					preserved,
				},
			},
			version: "new-version",
		},
		{
			name: "different version declines",
			machine: &fly.Machine{
				State:   fly.MachineStateStopped,
				Version: "other-version",
				Config:  &fly.MachineConfig{},
				Events:  []*fly.MachineEvent{preserved},
			},
			version: "new-version",
		},
		{
			name: "non-flyd event declines",
			machine: &fly.Machine{
				State:   fly.MachineStateStopped,
				Version: "new-version",
				Config:  &fly.MachineConfig{},
				Events: []*fly.MachineEvent{
					{Type: "update", Status: fly.MachineStateStopped, Source: "user"},
				},
			},
			version: "new-version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isPreservedStoppedUpdate(tc.machine, tc.version))
		})
	}
}

func TestSupportsPreservedStoppedUpdate(t *testing.T) {
	require.True(t, supportsPreservedStoppedUpdate("canary", fly.MachineStateStarted))
	require.True(t, supportsPreservedStoppedUpdate("rolling", "starting"))
	require.False(t, supportsPreservedStoppedUpdate("canary", "failed"))
	require.False(t, supportsPreservedStoppedUpdate("immediate", fly.MachineStateStarted))
}

func TestWaitForMachineAcceptsPreservedStoppedUpdate(t *testing.T) {
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	ios, _, _, _ := iostreams.Test()
	const version = "01G6R2TQGS41MBQTCA55X8ZCZW"
	startedWaitObserved := make(chan struct{})
	var sawMachineGet atomic.Bool
	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
		Transport: updateOutcomeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			state := req.URL.Query().Get("state")
			if state == fly.MachineStateStarted {
				close(startedWaitObserved)
				<-req.Context().Done()

				return nil, req.Context().Err()
			}
			if state == fly.MachineStateStopped {
				select {
				case <-startedWaitObserved:
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}

				return machineResponse(req, ""), nil
			}

			sawMachineGet.Store(true)

			return machineResponse(req, `{"id":"machine-id","version":"`+version+`","state":"stopped","events":[{"type":"update","status":"stopped","source":"flyd"}],"config":{}}`), nil
		}),
	})
	require.NoError(t, err)
	entry := &machineUpdateEntry{
		leasableMachine: machine.NewLeasableMachine(client, ios, "app", &fly.Machine{
			ID:      "machine-id",
			Version: version,
		}, false),
		launchInput: &fly.LaunchMachineInput{},
	}
	md := &machineDeployment{
		app:             &flaps.App{Name: "app"},
		io:              ios,
		flapsClient:     client,
		strategy:        "canary",
		waitTimeout:     time.Second,
		skipSmokeChecks: true,
	}
	ctx := iostreams.NewContext(context.Background(), ios)
	line := statuslogger.Create(ctx, 1, false).Line(0)

	require.NoError(t, md.waitForMachine(ctx, entry, fly.MachineStateStarted, line))
	require.True(t, sawMachineGet.Load())
}

func TestWaitForStartedOrPreservedStoppedUpdateDeclinesNewerLifecycleEvent(t *testing.T) {
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	ios, _, _, _ := iostreams.Test()
	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
		Transport: updateOutcomeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Query().Get("state") {
			case fly.MachineStateStarted:
				<-req.Context().Done()

				return nil, req.Context().Err()
			case fly.MachineStateStopped:
				return machineResponse(req, ""), nil
			default:
				return machineResponse(req, `{"id":"machine-id","version":"new-version","state":"stopped","events":[{"type":"exit","status":"stopped","source":"flyd"},{"type":"update","status":"stopped","source":"flyd"}],"config":{}}`), nil
			}
		}),
	})
	require.NoError(t, err)
	md := &machineDeployment{
		app:         &flaps.App{Name: "app"},
		flapsClient: client,
		strategy:    "canary",
	}
	lm := machine.NewLeasableMachine(client, ios, "app", &fly.Machine{
		ID:      "machine-id",
		Version: "new-version",
	}, false)

	preservedStopped, err := md.waitForStartedOrPreservedStoppedUpdate(
		context.Background(),
		lm,
		fly.MachineStateStarted,
		20*time.Millisecond,
	)

	require.False(t, preservedStopped)
	require.Error(t, err)
}

func TestUpdateMachineWChecksAcceptsPreservedStoppedUpdate(t *testing.T) {
	for _, strategy := range []string{"canary", "rolling"} {
		t.Run(strategy, func(t *testing.T) {
			testUpdateMachineWChecksAcceptsPreservedStoppedUpdate(t, strategy)
		})
	}
}

func testUpdateMachineWChecksAcceptsPreservedStoppedUpdate(t *testing.T, strategy string) {
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	const version = "01G6R2TQGS41MBQTCA55X8ZCZW"
	oldMachine := &fly.Machine{
		ID:         "machine-id",
		State:      fly.MachineStateStarted,
		LeaseNonce: "lease-nonce",
		HostStatus: fly.HostStatusOk,
		Config:     &fly.MachineConfig{Image: "image-v1"},
	}
	newMachine := &fly.Machine{
		ID:         oldMachine.ID,
		State:      fly.MachineStateStarted,
		HostStatus: fly.HostStatusOk,
		Config:     &fly.MachineConfig{Image: "image-v2"},
	}

	var sawSkipLaunch, sawMachineGet atomic.Bool
	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
		Transport: updateOutcomeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost {
				var input fly.LaunchMachineInput
				require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
				sawSkipLaunch.Store(input.SkipLaunch)

				return machineResponse(req, `{"id":"machine-id","version":"`+version+`","state":"created","config":{"image":"image-v2"}}`), nil
			}

			state := req.URL.Query().Get("state")
			if state != "" {
				if state == fly.MachineStateStopped {
					return machineResponse(req, ""), nil
				}
				<-req.Context().Done()

				return nil, req.Context().Err()
			}

			sawMachineGet.Store(true)

			return machineResponse(req, `{"id":"machine-id","version":"`+version+`","state":"stopped","events":[{"type":"update","status":"stopped","source":"flyd"}],"config":{"image":"image-v2"}}`), nil
		}),
	})
	require.NoError(t, err)
	md := &machineDeployment{
		app:         &flaps.App{Name: "app"},
		appConfig:   &appconfig.Config{AppName: "app"},
		flapsClient: client,
		io:          ios,
		strategy:    strategy,
		waitTimeout: 2 * time.Second,
	}
	line := statuslogger.Create(ctx, 1, false).Line(0)

	require.NoError(t, md.updateMachineWChecks(ctx, oldMachine, newMachine, false, oldMachine.State, line, ios, &healthcheckResult{}))
	require.False(t, sawSkipLaunch.Load(), "observed outcome must not rewrite retry-stable launch intent")
	require.True(t, sawMachineGet.Load())
}

func machineResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

// TestUpdateMachinesWRecoveryJudgesPreservedStoppedAgainstLaunchBasis checks that
// a stopped update outcome is judged against the read that decided skip_launch
// (originalAppState), not against the read the update was applied to
// (oldAppState). The canary strategy's second phase and deploy retries pass a
// later re-read as oldAppState.
func TestUpdateMachinesWRecoveryJudgesPreservedStoppedAgainstLaunchBasis(t *testing.T) {
	const id = "launch-basis-machine"

	tests := []struct {
		name           string
		strategy       string
		launchBasis    string // state at the launch-basis read; "" if the Machine is not in it
		reread         string // state at the read the update is applied to
		outcome        string // state flyd leaves the Machine in
		wantSkipLaunch bool
		wantPreserved  bool
	}{
		{
			name:          "canary started at both reads",
			strategy:      "canary",
			launchBasis:   fly.MachineStateStarted,
			reread:        fly.MachineStateStarted,
			outcome:       fly.MachineStateStopped,
			wantPreserved: true,
		},
		{
			name:          "rolling started at both reads",
			strategy:      "rolling",
			launchBasis:   fly.MachineStateStarted,
			reread:        fly.MachineStateStarted,
			outcome:       fly.MachineStateStopped,
			wantPreserved: true,
		},
		{
			name:          "canary stopped after the launch-basis read",
			strategy:      "canary",
			launchBasis:   fly.MachineStateStarted,
			reread:        fly.MachineStateStopped,
			outcome:       fly.MachineStateStopped,
			wantPreserved: true,
		},
		{
			name:           "stopped at the launch-basis read is a skip-launch update",
			strategy:       "canary",
			launchBasis:    fly.MachineStateStopped,
			reread:         fly.MachineStateStarted,
			outcome:        fly.MachineStateStopped,
			wantSkipLaunch: true,
		},
		{
			name:        "update started a Machine stopped after the launch-basis read",
			strategy:    "canary",
			launchBasis: fly.MachineStateStarted,
			reread:      fly.MachineStateStopped,
			outcome:     fly.MachineStateStarted,
		},
		{
			name:          "Machine missing from the launch-basis read uses the re-read",
			strategy:      "canary",
			reread:        fly.MachineStateStarted,
			outcome:       fly.MachineStateStopped,
			wantPreserved: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := &AppState{}
			if tc.launchBasis != "" {
				original.Machines = []*fly.Machine{listedMachine(id, tc.launchBasis)}
			}
			reread := listedMachine(id, tc.reread)
			reread.LeaseNonce = "lease-nonce"
			api := newFakeMachinesAPI(t, nil, updatedMachine(id, tc.outcome))
			ctx, md, out := newFakeAPIDeployment(t, tc.strategy, api)

			err := md.updateMachinesWRecovery(ctx, original, &AppState{Machines: []*fly.Machine{reread}}, deployedState(id), nil, updateMachineSettings{
				skipSmokeChecks:      true,
				skipLeaseAcquisition: true,
			})

			require.NoError(t, err)
			require.Equal(t, tc.wantSkipLaunch, api.skipLaunch(id), "skip_launch sent with the update")
			if tc.wantPreserved {
				require.Contains(t, out.String(), "was updated and left stopped")
			} else {
				require.NotContains(t, out.String(), "was updated and left stopped")
				require.Contains(t, out.String(), "is now in a good state")
			}
		})
	}
}

// TestUpdateMachinesWRecoveryImmediateSkipsUpdateOutcome checks that the
// immediate strategy, which skips health and smoke checks, never waits for an
// update outcome, so the launch basis does not apply to it.
func TestUpdateMachinesWRecoveryImmediateSkipsUpdateOutcome(t *testing.T) {
	const id = "immediate-machine"

	reread := listedMachine(id, fly.MachineStateStopped)
	reread.LeaseNonce = "lease-nonce"
	api := newFakeMachinesAPI(t, nil, updatedMachine(id, fly.MachineStateStopped))
	ctx, md, _ := newFakeAPIDeployment(t, "immediate", api)
	original := &AppState{Machines: []*fly.Machine{listedMachine(id, fly.MachineStateStarted)}}

	err := md.updateMachinesWRecovery(ctx, original, &AppState{Machines: []*fly.Machine{reread}}, deployedState(id), nil, updateMachineSettings{
		skipHealthChecks:     true,
		skipSmokeChecks:      true,
		skipLeaseAcquisition: true,
	})

	require.NoError(t, err)
	require.Zero(t, api.waits(), "wait requests")
	require.Zero(t, api.gets(id), "Machine reads")
}

// TestUpdateMachinesWRecoveryRetryAcceptsPreservedStoppedUpdate checks the retry
// path: the first update fails, the proxy stops the Machine before the retry's
// re-read, and flyd keeps it stopped on the retried update.
func TestUpdateMachinesWRecoveryRetryAcceptsPreservedStoppedUpdate(t *testing.T) {
	const id = "retry-machine"

	started := listedMachine(id, fly.MachineStateStarted)
	started.LeaseNonce = "lease-nonce"
	api := newFakeMachinesAPI(t, [][]*fly.Machine{{listedMachine(id, fly.MachineStateStopped)}}, updatedMachine(id, fly.MachineStateStopped))
	api.failedUpdates = 1
	ctx, md, out := newFakeAPIDeployment(t, "rolling", api)
	md.deployRetries = 1
	original := &AppState{Machines: []*fly.Machine{started}}

	err := md.updateMachinesWRecovery(ctx, original, original, deployedState(id), nil, updateMachineSettings{
		pushForward:          true,
		skipSmokeChecks:      true,
		skipLeaseAcquisition: true,
	})

	require.NoError(t, err)
	require.Equal(t, 2, api.updates(id), "update requests")
	require.False(t, api.skipLaunch(id), "skip_launch sent with the retried update")
	require.Contains(t, out.String(), "was updated and left stopped")
}

// TestCanaryAcceptsPreservedStoppedUpdateAfterReread reproduces a canary deploy
// in which the proxy autostops a Machine after the first read and before the
// re-read that the second phase runs against. flyd keeps the Machine stopped on
// update, and the deploy must accept that outcome instead of waiting for health
// checks until --wait-timeout.
func TestCanaryAcceptsPreservedStoppedUpdateAfterReread(t *testing.T) {
	const canaryID, autostoppedID = "canary-machine", "autostopped-machine"

	firstRead := []*fly.Machine{
		listedMachine(canaryID, fly.MachineStateStarted),
		listedMachine(autostoppedID, fly.MachineStateStarted),
	}
	canaryUpdated := listedMachine(canaryID, fly.MachineStateStarted)
	canaryUpdated.Config = deployedConfig()
	reread := []*fly.Machine{canaryUpdated, listedMachine(autostoppedID, fly.MachineStateStopped)}
	api := newFakeMachinesAPI(t, [][]*fly.Machine{firstRead, reread},
		updatedMachine(canaryID, fly.MachineStateStarted),
		updatedMachine(autostoppedID, fly.MachineStateStopped),
	)
	ctx, md, out := newFakeAPIDeployment(t, "canary", api)
	var entries []*machineUpdateEntry
	for _, m := range firstRead {
		entries = append(entries, &machineUpdateEntry{
			leasableMachine: machine.NewLeasableMachine(md.flapsClient, md.io, "app", listedMachine(m.ID, m.State), false),
			launchInput:     &fly.LaunchMachineInput{Config: deployedConfig()},
		})
	}

	require.NoError(t, md.updateExistingMachinesWRecovery(ctx, entries))
	require.Equal(t, 1, api.updates(canaryID), "canary update requests")
	require.Equal(t, 1, api.updates(autostoppedID), "update requests")
	require.False(t, api.skipLaunch(autostoppedID), "skip_launch sent with the update")
	require.Contains(t, out.String(), "Machine "+autostoppedID+" was updated and left stopped")
}

const updatedVersion = "01G6R2TQGS41MBQTCA55X8ZCZW"

// listedMachine returns a Machine as a read before its update reports it.
func listedMachine(id, state string) *fly.Machine {
	return &fly.Machine{
		ID:         id,
		State:      state,
		HostStatus: fly.HostStatusOk,
		Config:     &fly.MachineConfig{Image: "image-v1"},
	}
}

// updatedMachine returns a Machine as flyd reports it after an update that
// leaves it in state.
func updatedMachine(id, state string) *fly.Machine {
	m := &fly.Machine{ID: id, State: state, Version: updatedVersion, Config: deployedConfig()}
	switch state {
	case fly.MachineStateStopped:
		m.Events = []*fly.MachineEvent{{Type: "update", Status: fly.MachineStateStopped, Source: "flyd"}}
	case fly.MachineStateStarted:
		m.Checks = []*fly.MachineCheckStatus{{Name: "alive", Status: fly.Passing}}
	}

	return m
}

// deployedConfig returns the config being deployed. It has a health check, so a
// deploy that does not accept a stopped outcome waits for the check to pass.
func deployedConfig() *fly.MachineConfig {
	return &fly.MachineConfig{
		Image:  "image-v2",
		Checks: map[string]fly.MachineCheck{"alive": {Type: fly.Pointer("tcp"), Port: fly.Pointer(8080)}},
	}
}

// deployedState returns the desired state of a started Machine id.
func deployedState(id string) *AppState {
	return &AppState{Machines: []*fly.Machine{{
		ID:         id,
		State:      fly.MachineStateStarted,
		HostStatus: fly.HostStatusOk,
		Config:     deployedConfig(),
	}}}
}

// newFakeAPIDeployment returns a deployment that sends its Machines API requests
// to api, a context for it, and the buffer that receives its status lines.
func newFakeAPIDeployment(t *testing.T, strategy string, api *fakeMachinesAPI) (context.Context, *machineDeployment, *bytes.Buffer) {
	t.Helper()
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	ios, _, out, _ := iostreams.Test()
	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{Transport: api})
	require.NoError(t, err)
	md := &machineDeployment{
		app:               &flaps.App{Name: "app"},
		appConfig:         &appconfig.Config{AppName: "app"},
		flapsClient:       client,
		io:                ios,
		colorize:          ios.ColorScheme(),
		strategy:          strategy,
		maxUnavailable:    1,
		waitTimeout:       2 * time.Second,
		leaseTimeout:      13 * time.Second,
		leaseDelayBetween: time.Minute,
		skipSmokeChecks:   true,
	}

	return iostreams.NewContext(context.Background(), ios), md, out
}

// fakeMachinesAPI answers the Machines API requests of a deploy. List returns
// the next of reads, repeating the last. After an update, Wait and Get report
// the Machine as updated.
type fakeMachinesAPI struct {
	t             *testing.T
	reads         [][]*fly.Machine
	updated       map[string]*fly.Machine
	failedUpdates int // update requests answered with 409 before the first success

	mu             sync.Mutex
	lists          int
	updateCount    map[string]int
	lastSkipLaunch map[string]bool
	getCount       map[string]int
	waitCount      int
}

func newFakeMachinesAPI(t *testing.T, reads [][]*fly.Machine, updated ...*fly.Machine) *fakeMachinesAPI {
	t.Helper()

	api := &fakeMachinesAPI{
		t:              t,
		reads:          reads,
		updated:        make(map[string]*fly.Machine),
		updateCount:    make(map[string]int),
		lastSkipLaunch: make(map[string]bool),
		getCount:       make(map[string]int),
	}
	for _, m := range updated {
		api.updated[m.ID] = m
		// healthChecksPassed is package state keyed by Machine ID.
		t.Cleanup(func() { healthChecksPassed.Delete(m.ID) })
	}

	return api
}

func (f *fakeMachinesAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	path, ok := strings.CutPrefix(req.URL.Path, "/v1/apps/app/machines")
	id, action, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	switch {
	case !ok:
		// Not a request for the app's Machines.
	case req.Method == http.MethodGet && id == "":
		return f.list(req)
	case req.Method == http.MethodPost && action == "":
		return f.update(req, id)
	case req.Method == http.MethodGet && action == "wait":
		return f.wait(req, id)
	case req.Method == http.MethodGet && action == "":
		return f.get(req, id)
	case req.Method == http.MethodPost && action == "lease":
		return machineResponse(req, `{"status":"success","data":{"nonce":"lease-nonce"}}`), nil
	case req.Method == http.MethodDelete && action == "lease":
		return machineResponse(req, ""), nil
	}
	f.t.Errorf("unexpected request %s %s", req.Method, req.URL)

	return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
}

func (f *fakeMachinesAPI) list(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.reads) == 0 {
		return nil, fmt.Errorf("unexpected list request")
	}
	read := f.reads[min(f.lists, len(f.reads)-1)]
	f.lists++

	return jsonResponse(req, read)
}

func (f *fakeMachinesAPI) update(req *http.Request, id string) (*http.Response, error) {
	var input fly.LaunchMachineInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.updateCount[id]++
	f.lastSkipLaunch[id] = input.SkipLaunch
	if f.failedUpdates > 0 {
		f.failedUpdates--
		resp := machineResponse(req, `{"error":"conflict"}`)
		resp.StatusCode = http.StatusConflict

		return resp, nil
	}

	return jsonResponse(req, &fly.Machine{ID: id, State: "created", Version: f.updated[id].Version, Config: input.Config})
}

// wait answers a wait for the Machine's updated state and blocks a wait for any
// other state until the request is canceled.
func (f *fakeMachinesAPI) wait(req *http.Request, id string) (*http.Response, error) {
	f.mu.Lock()
	f.waitCount++
	reached := f.updated[id].State == req.URL.Query().Get("state")
	f.mu.Unlock()

	if !reached {
		<-req.Context().Done()

		return nil, req.Context().Err()
	}

	return machineResponse(req, ""), nil
}

func (f *fakeMachinesAPI) get(req *http.Request, id string) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.getCount[id]++

	return jsonResponse(req, f.updated[id])
}

func (f *fakeMachinesAPI) updates(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.updateCount[id]
}

func (f *fakeMachinesAPI) skipLaunch(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lastSkipLaunch[id]
}

func (f *fakeMachinesAPI) gets(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.getCount[id]
}

func (f *fakeMachinesAPI) waits() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.waitCount
}

func jsonResponse(req *http.Request, v any) (*http.Response, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return machineResponse(req, string(body)), nil
}
