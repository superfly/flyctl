package deploy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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
		name     string
		machine  *fly.Machine
		instance string
		want     bool
	}{
		{
			name: "matching current update event",
			machine: &fly.Machine{
				State:      fly.MachineStateStopped,
				InstanceID: "new-version",
				Config:     &fly.MachineConfig{},
				Events:     []*fly.MachineEvent{preserved},
			},
			instance: "new-version",
			want:     true,
		},
		{
			name:     "missing machine declines",
			instance: "new-version",
		},
		{
			name: "missing config declines",
			machine: &fly.Machine{
				State:      fly.MachineStateStopped,
				InstanceID: "new-version",
				Events:     []*fly.MachineEvent{preserved},
			},
			instance: "new-version",
		},
		{
			name: "scheduled update declines",
			machine: &fly.Machine{
				State:      fly.MachineStateStopped,
				InstanceID: "new-version",
				Config:     &fly.MachineConfig{Schedule: "daily"},
				Events:     []*fly.MachineEvent{preserved},
			},
			instance: "new-version",
		},
		{
			name: "newer lifecycle event declines",
			machine: &fly.Machine{
				State:      fly.MachineStateStopped,
				InstanceID: "new-version",
				Config:     &fly.MachineConfig{},
				Events: []*fly.MachineEvent{
					{Type: "exit", Status: fly.MachineStateStopped, Source: "flyd"},
					preserved,
				},
			},
			instance: "new-version",
		},
		{
			name: "different version declines",
			machine: &fly.Machine{
				State:      fly.MachineStateStopped,
				InstanceID: "other-version",
				Config:     &fly.MachineConfig{},
				Events:     []*fly.MachineEvent{preserved},
			},
			instance: "new-version",
		},
		{
			name: "non-flyd event declines",
			machine: &fly.Machine{
				State:      fly.MachineStateStopped,
				InstanceID: "new-version",
				Config:     &fly.MachineConfig{},
				Events: []*fly.MachineEvent{
					{Type: "update", Status: fly.MachineStateStopped, Source: "user"},
				},
			},
			instance: "new-version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isPreservedStoppedUpdate(tc.machine, tc.instance))
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
	const instanceID = "01G6R2TQGS41MBQTCA55X8ZCZW"
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

			return machineResponse(req, `{"id":"machine-id","instance_id":"`+instanceID+`","state":"stopped","events":[{"type":"update","status":"stopped","source":"flyd"}],"config":{}}`), nil
		}),
	})
	require.NoError(t, err)
	entry := &machineUpdateEntry{
		leasableMachine: machine.NewLeasableMachine(client, ios, "app", &fly.Machine{
			ID:         "machine-id",
			InstanceID: instanceID,
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
				return machineResponse(req, `{"id":"machine-id","instance_id":"new-version","state":"stopped","events":[{"type":"exit","status":"stopped","source":"flyd"},{"type":"update","status":"stopped","source":"flyd"}],"config":{}}`), nil
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
		ID:         "machine-id",
		InstanceID: "new-version",
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
	const instanceID = "01G6R2TQGS41MBQTCA55X8ZCZW"
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

				return machineResponse(req, `{"id":"machine-id","instance_id":"`+instanceID+`","state":"created","config":{"image":"image-v2"}}`), nil
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

			return machineResponse(req, `{"id":"machine-id","instance_id":"`+instanceID+`","state":"stopped","events":[{"type":"update","status":"stopped","source":"flyd"}],"config":{"image":"image-v2"}}`), nil
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

	require.NoError(t, md.updateMachineWChecks(ctx, oldMachine, newMachine, false, line, ios, &healthcheckResult{}))
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
