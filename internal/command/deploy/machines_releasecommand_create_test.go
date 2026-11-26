package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
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

func TestCreateReleaseCommandMachine(t *testing.T) {
	const (
		appName      = "test-app"
		machineID    = "release-fixture"
		machinesPath = "/v1/apps/" + appName + "/machines"
		attemptKey   = "fly_release_command_attempt"
		nonce        = "fixture-lease"
	)
	for _, tc := range []struct {
		name          string
		launchStatus  int
		listMode      string
		afterLease    bool
		leaseVersion  string
		leaseDuration time.Duration
		startDelay    time.Duration
		buildRefusal  bool
		change        func(*fly.Machine)
		parentTimeout time.Duration
		waitTimeout   time.Duration
		wantCalls     []string
		wantCreated   bool
		noReleaseID   bool
		zeroWait      bool
		startStatus   int
		startResponse any
		leaseStatus   int
		releaseStatus int
		startError    bool
		launchError   error
	}{
		{name: "ordinary create", launchStatus: 200, wantCalls: []string{"create", "wait"}, wantCreated: true},
		{name: "empty 408 after create", launchStatus: 408, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "release"}, wantCreated: true},
		{name: "late visibility", launchStatus: 408, listMode: "late", wantCalls: []string{"create", "list", "list", "get", "lease", "get", "start", "release"}, wantCreated: true},
		{name: "duplicate matches", launchStatus: 408, listMode: "duplicate", wantCalls: []string{"create", "list"}},
		{name: "list returned another attempt", launchStatus: 408, listMode: "wrong-attempt", wantCalls: []string{"create", "list"}},
		{name: "list already shows an exit", launchStatus: 408, listMode: "already-ran", wantCalls: []string{"create", "list"}},
		{name: "list fails", launchStatus: 408, listMode: "error", wantCalls: []string{"create", "list"}},
		{name: "changed version between list and fresh read", launchStatus: 408, change: func(m *fly.Machine) { m.InstanceID = "new-version" }, wantCalls: []string{"create", "list", "get"}},
		{name: "changed metadata", launchStatus: 408, change: func(m *fly.Machine) { m.Config.Metadata[attemptKey] = "different-attempt" }, wantCalls: []string{"create", "list", "get"}},
		{name: "competing start after first read", launchStatus: 408, afterLease: true, change: func(m *fly.Machine) {
			m.Events = append([]*fly.MachineEvent{{ID: "exit", Type: "exit"}, {ID: "start", Type: "start", Status: "started"}}, m.Events...)
		}, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "release"}},
		{name: "lease acquired on changed version", launchStatus: 408, leaseVersion: "different-version", wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "release"}},
		{name: "expired lease", launchStatus: 408, leaseDuration: time.Second, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "release"}},
		{name: "remaining recovery deadline during start", launchStatus: 408, listMode: "late", waitTimeout: 1500 * time.Millisecond, startDelay: time.Second, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "list", "get", "lease", "get", "start"}},
		{name: "start conflict", launchStatus: 408, startStatus: 409, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start"}},
		{name: "unavailable lease", launchStatus: 408, leaseStatus: 409, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease"}},
		{name: "ambiguous start 503", launchStatus: 408, startStatus: 503, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start"}},
		{name: "ambiguous release 503 after successful start", launchStatus: 408, releaseStatus: 503, wantCreated: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "release"}},
		{name: "uncertain start result", launchStatus: 408, startStatus: 408, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start"}},
		{name: "ambiguous lease 503", launchStatus: 408, leaseStatus: 503, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease"}},
		{name: "ambiguous lease 408", launchStatus: 408, leaseStatus: 408, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease"}},
		{name: "previously started success response", launchStatus: 408, startResponse: &fly.MachineStartResponse{Status: "success", PreviousState: fly.MachineStateStarted}, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "release"}},
		{name: "missing previous state", launchStatus: 408, startResponse: &fly.MachineStartResponse{Status: "success"}, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "release"}},
		{name: "null start response", launchStatus: 408, startResponse: json.RawMessage("null"), wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "release"}},
		{name: "creation events changed under lease", launchStatus: 408, afterLease: true, change: func(m *fly.Machine) {
			replacement := *m.Events[1]
			replacement.ID = "different-creation-event"
			m.Events = append([]*fly.MachineEvent(nil), m.Events...)
			m.Events[1] = &replacement
		}, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "release"}},
		{name: "explicit build-not-ready refusal", launchStatus: 408, buildRefusal: true, wantCreated: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "get", "start", "release"}},
		{name: "changed instance between lists", launchStatus: 408, listMode: "change-version", wantCalls: []string{"create", "list", "list"}},
		{name: "changed machine between lists", launchStatus: 408, listMode: "change-machine", wantCalls: []string{"create", "list", "list"}},
		{name: "transport error", launchError: errors.New("test connection reset"), wantCalls: []string{"create"}},
		{name: "no release identity", launchStatus: 408, noReleaseID: true, wantCalls: []string{"create"}},
		{name: "zero wait budget", launchStatus: 408, zeroWait: true, wantCalls: []string{"create"}},
		{name: "ambiguous create 503", launchStatus: 503, wantCalls: []string{"create"}},
		{name: "ambiguous create 502", launchStatus: 502, wantCalls: []string{"create"}},
		{name: "ambiguous start 502", launchStatus: 408, startStatus: 502, wantCreated: true, startError: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start"}},
		{name: "ambiguous release 502 after successful start", launchStatus: 408, releaseStatus: 502, wantCreated: true, wantCalls: []string{"create", "list", "get", "lease", "get", "start", "release"}},
		{name: "temporary transport failure", launchError: &net.DNSError{Err: "temporary failure", Name: "fixture.invalid", IsTemporary: true}, wantCalls: []string{"create"}},
		{name: "other HTTP failure", launchStatus: 500, wantCalls: []string{"create"}},
		{name: "recovery deadline", launchStatus: 408, listMode: "blocked", waitTimeout: 100 * time.Millisecond, wantCalls: []string{"create", "list"}},
		{name: "caller deadline", launchStatus: 408, listMode: "blocked", parentTimeout: 100 * time.Millisecond, wantCalls: []string{"create", "list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ctx, cleanup := statuslogger.SingleLine(iostreams.NewContext(t.Context(), ios), false)
			defer cleanup(false)
			if tc.parentTimeout != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.parentTimeout)
				defer cancel()
			}

			var calls []string
			var candidate fly.Machine
			listCalls := 0
			leaseAcquired := false
			startCalls := 0
			client, err := flaps.NewWithOptions(ctx, flaps.NewClientOpts{Transport: releaseStartTransport(func(r *http.Request) (*http.Response, error) {
				status := http.StatusOK
				var body any
				switch r.Method + " " + r.URL.Path {
				case "POST " + machinesPath:
					calls = append(calls, "create")
					if tc.launchError != nil {
						return nil, tc.launchError
					}
					var input fly.LaunchMachineInput
					require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
					require.True(t, input.SkipLaunch)
					require.NotEmpty(t, input.Config.Metadata[attemptKey])
					candidate = fly.Machine{
						ID: machineID, InstanceID: "original-version", State: fly.MachineStateStopped, Config: input.Config,
						Events: []*fly.MachineEvent{
							{ID: "stop", Type: "update", Status: "stopped"},
							{ID: "create", Type: "launch", Status: "created"},
							{ID: "pending", Type: "launch", Status: "pending"},
						},
					}
					status = tc.launchStatus
					if status == http.StatusOK {
						body = candidate
					}
				case "GET " + machinesPath:
					calls = append(calls, "list")
					require.Equal(t, candidate.Config.Metadata[attemptKey], r.URL.Query().Get("metadata."+attemptKey))
					listCalls++
					body = []fly.Machine{candidate}
					switch tc.listMode {
					case "late":
						if listCalls == 1 {
							body = []fly.Machine{}
						}
					case "change-version", "change-machine":
						visible := candidate
						if listCalls == 1 {
							visible.State = fly.MachineStateCreated
						} else if tc.listMode == "change-version" {
							visible.InstanceID = "other-instance"
						} else {
							visible.ID = "other-machine"
						}
						body = []fly.Machine{visible}
					case "wrong-attempt":
						listed := candidate
						config := *listed.Config
						config.Metadata = maps.Clone(config.Metadata)
						config.Metadata[attemptKey] = "other-attempt"
						listed.Config = &config
						body = []fly.Machine{listed}
					case "already-ran":
						listed := candidate
						listed.Events = append([]*fly.MachineEvent{{ID: "exit", Type: "exit"}}, listed.Events...)
						body = []fly.Machine{listed}
					case "duplicate":
						duplicate := candidate
						duplicate.ID = "second-machine"
						body = []fly.Machine{candidate, duplicate}
					case "error":
						status = http.StatusInternalServerError
						body = nil
					case "blocked":
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
				case "GET " + machinesPath + "/" + machineID:
					calls = append(calls, "get")
					fresh := candidate
					config := *candidate.Config
					config.Metadata = maps.Clone(config.Metadata)
					fresh.Config = &config
					if tc.change != nil && tc.afterLease == leaseAcquired {
						tc.change(&fresh)
					}
					body = fresh
				case "POST " + machinesPath + "/" + machineID + "/lease":
					calls = append(calls, "lease")
					require.Equal(t, "120", r.URL.Query().Get("ttl"))
					require.Equal(t, []string{""}, r.Header[flaps.NonceHeader])
					if tc.leaseStatus != 0 {
						status = tc.leaseStatus
					} else {
						leaseAcquired = true
						version := candidate.InstanceID
						if tc.leaseVersion != "" {
							version = tc.leaseVersion
						}
						duration := 2 * time.Minute
						if tc.leaseDuration != 0 {
							duration = tc.leaseDuration
						}
						body = fly.MachineLease{Status: "success", Data: &fly.MachineLeaseData{Nonce: nonce, Version: version, ExpiresAt: time.Now().Add(duration).Unix()}}
					}
				case "POST " + machinesPath + "/" + machineID + "/start":
					calls = append(calls, "start")
					startCalls++
					require.Equal(t, []string{nonce}, r.Header[flaps.NonceHeader])
					if tc.startDelay > 0 {
						select {
						case <-time.After(tc.startDelay):
						case <-r.Context().Done():
							return nil, r.Context().Err()
						}
					}
					if tc.buildRefusal && startCalls == 1 {
						status = http.StatusPreconditionFailed
						body = map[string]string{"error": "failed_precondition: machine build not ready, retry the start request"}
					} else if tc.startStatus != 0 {
						status = tc.startStatus
					} else if tc.startResponse != nil {
						body = tc.startResponse
					} else {
						body = fly.MachineStartResponse{Status: "success", PreviousState: fly.MachineStateStopped}
					}
				case "DELETE " + machinesPath + "/" + machineID + "/lease":
					calls = append(calls, "release")
					require.NoError(t, r.Context().Err())
					require.Equal(t, []string{nonce}, r.Header[flaps.NonceHeader])
					if tc.releaseStatus != 0 {
						status = tc.releaseStatus
					}
				case "GET " + machinesPath + "/" + machineID + "/wait":
					calls = append(calls, "wait")
					require.Equal(t, fly.MachineStateStopped, r.URL.Query().Get("state"))
				default:
					t.Fatalf("unexpected Machines API request %s %s", r.Method, r.URL)
				}
				var payload []byte
				if body != nil {
					var err error
					payload, err = json.Marshal(body)
					require.NoError(t, err)
				}
				return &http.Response{
					StatusCode: status, Header: http.Header{"Fly-Request-Id": []string{"fixture-request"}},
					Body: io.NopCloser(strings.NewReader(string(payload))), Request: r,
				}, nil
			})})
			require.NoError(t, err)

			waitTimeout := tc.waitTimeout
			if waitTimeout == 0 {
				waitTimeout = 3 * time.Second
			}
			if tc.zeroWait {
				waitTimeout = 0
			}
			releaseID := "fixture-release"
			if tc.noReleaseID {
				releaseID = ""
			}
			md := machineDeployment{
				app:         &flaps.App{Name: appName},
				appConfig:   &appconfig.Config{AppName: appName, Deploy: &appconfig.Deploy{ReleaseCommand: "true"}},
				flapsClient: client,
				img:         "registry.example.invalid/test-app:fixture",
				io:          ios, colorize: ios.ColorScheme(),
				machineSet:            machine.NewMachineSet(client, ios, appName, nil, false),
				releaseCommandMachine: machine.NewMachineSet(client, ios, appName, nil, false),
				releaseId:             releaseID,
				waitTimeout:           waitTimeout,
			}
			startRecovered, err := md.createReleaseCommandMachine(ctx)
			if tc.wantCreated {
				require.NoError(t, err)
				require.Len(t, md.releaseCommandMachine.GetMachines(), 1)
				require.Equal(t, machineID, md.releaseCommandMachine.GetMachines()[0].Machine().ID)
				if tc.launchStatus == http.StatusRequestTimeout {
					require.NotNil(t, startRecovered)
					err = startRecovered(ctx)
					if tc.startError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				} else {
					require.Nil(t, startRecovered)
				}
			} else if tc.launchError != nil {
				require.ErrorIs(t, err, tc.launchError)
				require.Nil(t, startRecovered)
			} else {
				require.Nil(t, startRecovered)
				var responseErr *flaps.FlapsError
				require.ErrorAs(t, err, &responseErr)
				require.Equal(t, tc.launchStatus, responseErr.ResponseStatusCode)
				require.Equal(t, "fixture-request", flaps.GetErrorRequestID(err))
				require.True(t, md.releaseCommandMachine.IsEmpty())
			}
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}
