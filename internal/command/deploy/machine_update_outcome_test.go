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
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

// Ensure readiness owns the state-wait operation even when a Machine intentionally stays stopped.
func TestWaitForMachineAcceptsPreservedStoppedUpdate(t *testing.T) {
	recorder := recordReadinessSpans(t)
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
	assertReadinessSpan(t, recorder)
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

// Ensure the recovery path also nests state checks beneath a dedicated readiness operation.
func TestUpdateMachineWChecksAcceptsPreservedStoppedUpdate(t *testing.T) {
	for _, strategy := range []string{"canary", "rolling"} {
		t.Run(strategy, func(t *testing.T) {
			testUpdateMachineWChecksAcceptsPreservedStoppedUpdate(t, strategy)
		})
	}
}

func testUpdateMachineWChecksAcceptsPreservedStoppedUpdate(t *testing.T, strategy string) {
	recorder := recordReadinessSpans(t)
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

	require.NoError(t, md.updateMachineWChecks(ctx, oldMachine, newMachine, false, line, ios, &healthcheckResult{}))
	assertReadinessSpan(t, recorder)
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

func recordReadinessSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	return recorder
}

func assertReadinessSpan(t *testing.T, recorder *tracetest.SpanRecorder) {
	t.Helper()
	var readiness, state sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch span.Name() {
		case "deployment_readiness":
			readiness = span
		case "wait_for_machine_state":
			state = span
		}
	}
	require.NotNil(t, readiness)
	require.NotNil(t, state)
	require.Equal(t, readiness.SpanContext().SpanID(), state.Parent().SpanID())
}
