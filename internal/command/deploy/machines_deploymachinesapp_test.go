package deploy

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/iostreams"
)

type deployRoundTripFunc func(*http.Request) (*http.Response, error)

func (f deployRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWaitForMachineUsesCanaryTargetState(t *testing.T) {
	t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

	ios, _, _, _ := iostreams.Test()
	const instanceID = "01G6R2TQGS41MBQTCA55X8ZCZW"
	var gotState, gotVersion string
	client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
		Transport: deployRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotState = req.URL.Query().Get("state")
			gotVersion = req.URL.Query().Get("version")

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
	})
	require.NoError(t, err)
	machineWithTarget := &fly.Machine{
		ID:          "machine-id",
		InstanceID:  instanceID,
		TargetState: fly.MachineStateStopped,
	}
	entry := &machineUpdateEntry{
		leasableMachine: machine.NewLeasableMachine(client, ios, "app", machineWithTarget, false),
		launchInput:     &fly.LaunchMachineInput{},
	}
	md := &machineDeployment{
		io:          ios,
		strategy:    "canary",
		waitTimeout: time.Second,
	}
	ctx := iostreams.NewContext(context.Background(), ios)
	line := statuslogger.Create(ctx, 1, false).Line(0)

	require.NoError(t, md.waitForMachine(ctx, entry, line))
	require.Equal(t, fly.MachineStateStopped, gotState)
	require.Equal(t, instanceID, gotVersion)
}

func TestWaitForMachineTargetStateExclusions(t *testing.T) {
	tests := []struct {
		name        string
		strategy    string
		target      string
		skipLaunch  bool
		wantState   string
		wantVersion string
		wantWait    bool
	}{
		{name: "rolling strategy", strategy: "rolling", target: fly.MachineStateStopped, wantState: fly.MachineStateStarted, wantWait: true},
		{name: "older server", strategy: "canary", wantState: fly.MachineStateStarted, wantWait: true},
		{name: "explicit skip launch", strategy: "canary", target: fly.MachineStateStopped, skipLaunch: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLY_FLAPS_BASE_URL", "http://flaps.test")

			ios, _, _, _ := iostreams.Test()
			var gotStates, gotVersions []string
			waitErr := errors.New("wait called")
			client, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{
				Transport: deployRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					gotStates = append(gotStates, req.URL.Query().Get("state"))
					gotVersions = append(gotVersions, req.URL.Query().Get("version"))

					return nil, waitErr
				}),
			})
			require.NoError(t, err)
			entry := &machineUpdateEntry{
				leasableMachine: machine.NewLeasableMachine(client, ios, "app", &fly.Machine{
					ID:          "machine-id",
					InstanceID:  "01G6R2TQGS41MBQTCA55X8ZCZW",
					TargetState: tc.target,
				}, false),
				launchInput: &fly.LaunchMachineInput{SkipLaunch: tc.skipLaunch},
			}
			md := &machineDeployment{
				io:              ios,
				appConfig:       appconfig.NewConfig(),
				strategy:        tc.strategy,
				waitTimeout:     20 * time.Millisecond,
				skipSmokeChecks: true,
			}
			ctx := iostreams.NewContext(context.Background(), ios)
			line := statuslogger.Create(ctx, 1, false).Line(0)

			err = md.waitForMachine(ctx, entry, line)
			if tc.wantWait {
				require.Error(t, err)
				require.NotEmpty(t, gotStates)
				require.Len(t, gotVersions, len(gotStates))
				for i := range gotStates {
					require.Equal(t, tc.wantState, gotStates[i])
					require.Equal(t, tc.wantVersion, gotVersions[i])
				}
			} else {
				require.NoError(t, err)
				require.Empty(t, gotStates)
				require.Empty(t, gotVersions)
			}
		})
	}
}

func TestUpdateExistingMachinesWRecovery(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	client := &mockFlapsClient{}
	client.machines = []*fly.Machine{{ID: "test-machine-id", LeaseNonce: "foobar"}}
	md := &machineDeployment{
		app:         &flaps.App{},
		io:          ios,
		colorize:    ios.ColorScheme(),
		flapsClient: client,
		strategy:    "canary",
	}

	ctx := context.Background()
	err := md.updateExistingMachinesWRecovery(ctx, nil)
	assert.NoError(t, err)

	err = md.updateExistingMachinesWRecovery(ctx, []*machineUpdateEntry{
		{
			leasableMachine: machine.NewLeasableMachine(client, ios, "", &fly.Machine{}, false),
			launchInput:     &fly.LaunchMachineInput{},
		},
	})
	assert.Error(t, err, "failed to find machine test-machine-id")
}

func TestDeployMachinesApp(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	client := &mockFlapsClient{}
	webClient := &mock.Client{
		GetAppLogsFunc: func(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error) {
			return nil, "", nil
		},
	}
	client.machines = []*fly.Machine{
		{ID: "m1", LeaseNonce: "m1-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
		{ID: "m2", LeaseNonce: "m2-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
		{ID: "m3", LeaseNonce: "m3-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
		{ID: "m4", LeaseNonce: "m4-lease", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "app"}}},
	}
	md := &machineDeployment{
		app:             &flaps.App{},
		io:              ios,
		colorize:        ios.ColorScheme(),
		flapsClient:     client,
		apiClient:       webClient,
		strategy:        "canary",
		appConfig:       &appconfig.Config{},
		machineSet:      machine.NewMachineSet(client, ios, "", client.machines, false),
		skipSmokeChecks: true,
		waitTimeout:     1 * time.Second,
	}

	// Shorten the NATS timeout since it's likely to require the fallback in CI
	natsConnectTimeout = md.waitTimeout

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, ios)
	ctx = flapsutil.NewContextWithClient(ctx, client)
	err := md.deployMachinesApp(ctx)
	assert.NoError(t, err)
}
