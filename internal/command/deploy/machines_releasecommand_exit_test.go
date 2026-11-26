package deploy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/iostreams"
)

func TestFinishReleaseCommandStart(t *testing.T) {
	const (
		abrupt       = "aborted: machine exited abruptly"
		machinesPath = "/v1/apps/test-app/machines"
		machineID    = "release-fixture"
	)
	for _, tc := range []struct {
		name           string
		status         int
		message        string
		mutate         func(*fly.Machine)
		getStatus      int
		historyMode    string
		ordinaryAnchor bool
		waitForDestroy bool
		wireVersion    string
		omitExitCode   bool
		nullExitCode   bool
		monitorEvent   bool
		wantRead       bool
		wantFastExit   bool
	}{
		{name: "successful start follows normal wait", status: 200},
		{name: "fast successful direct exit", status: 409, message: abrupt, wantRead: true, wantFastExit: true},
		{name: "fast successful monitor exit", status: 409, message: abrupt, wantRead: true, monitorEvent: true, wantFastExit: true},
		{name: "fast exit before started event", status: 409, message: abrupt, wantRead: true, wantFastExit: true, mutate: func(m *fly.Machine) { m.Events[1].Status = "starting" }},
		{name: "start progressed from starting to started", status: 409, message: abrupt, wantRead: true, wantFastExit: true, mutate: func(m *fly.Machine) {
			m.Events = []*fly.MachineEvent{m.Events[0], m.Events[1], {ID: "starting", Type: "start", Status: "starting", Timestamp: 150}, m.Events[2]}
		}},
		{name: "auto-destroy already underway", status: 409, message: abrupt, wantRead: true, wantFastExit: true, mutate: func(m *fly.Machine) {
			m.State = "destroying"
			m.Events = append([]*fly.MachineEvent{{ID: "destroy", Type: "destroy", Status: "destroying", Timestamp: 400}}, m.Events...)
		}},
		{name: "active state contradicts exit history", status: 409, message: abrupt, wantRead: true, mutate: func(m *fly.Machine) {
			m.State = fly.MachineStateStarted
		}},
		{name: "truncated history verifies recovered create", status: 409, message: abrupt, historyMode: "complete", wantRead: true, wantFastExit: true, waitForDestroy: true},
		{name: "truncated history verifies ordinary create", status: 409, message: abrupt, historyMode: "complete", ordinaryAnchor: true, wantRead: true, wantFastExit: true},
		{name: "ordinary create anchors original pending launch", status: 409, message: abrupt, ordinaryAnchor: true, wantRead: true, wantFastExit: true},
		{name: "event history differs from versioned read", status: 409, message: abrupt, historyMode: "different prefix", wantRead: true},
		{name: "event history contains other action before start", status: 409, message: abrupt, historyMode: "competing action", ordinaryAnchor: true, wantRead: true},
		{name: "event history reorders ordinary creation", status: 409, message: abrupt, historyMode: "reordered creation", ordinaryAnchor: true, wantRead: true},
		{name: "event history does not include creation anchor", status: 409, message: abrupt, historyMode: "missing anchor", wantRead: true},
		{name: "mismatched version alias", status: 409, message: abrupt, wireVersion: "wrong-version", wantRead: true},
		{name: "matching version alias", status: 409, message: abrupt, wireVersion: "release-version", wantRead: true, wantFastExit: true},
		{name: "ordinary conflict", status: 409, message: "lease currently held"},
		{name: "similar conflict text", status: 409, message: abrupt + " (old machine)"},
		{name: "same message with other status", status: 500, message: abrupt},
		{name: "read returned another machine", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.ID = "other-machine" }, wantRead: true},
		{name: "read returned another version", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.InstanceID = "other-version" }, wantRead: true},
		{name: "read does not follow redirect", status: 409, message: abrupt, getStatus: http.StatusTemporaryRedirect, wantRead: true},
		{name: "read found no machine", status: 409, message: abrupt, getStatus: 404, wantRead: true},
		{name: "exit predates latest start", status: 409, message: abrupt, mutate: func(m *fly.Machine) {
			m.Events = append([]*fly.MachineEvent{{ID: "newer-start", Type: "start", Status: "started", Timestamp: 400}}, m.Events...)
		}, wantRead: true},
		{name: "exit without start", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.Events = m.Events[:1] }, wantRead: true},
		{name: "exit without request", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.Events[0].Request = nil }, wantRead: true},
		{name: "exit without payload", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.Events[0].Request = &fly.MachineRequest{} }, wantRead: true},
		{name: "null exit code", status: 409, message: abrupt, nullExitCode: true, wantRead: true},
		{name: "populated exit without code", status: 409, message: abrupt, omitExitCode: true, wantRead: true},
		{name: "populated monitor exit without code", status: 409, message: abrupt, omitExitCode: true, monitorEvent: true, wantRead: true},
		{name: "wrong creation history", status: 409, message: abrupt, mutate: func(m *fly.Machine) {
			m.Events[2] = &fly.MachineEvent{ID: "unexpected-before-start", Type: "launch", Status: "created", Timestamp: 100}
		}, wantRead: true},
		{name: "empty exit payload", status: 409, message: abrupt, mutate: func(m *fly.Machine) { *m.Events[0].Request.ExitEvent = fly.MachineExitEvent{} }, wantRead: true},
		{name: "nonzero exit", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.Events[0].Request.ExitEvent.ExitCode = 17 }, wantRead: true},
		{name: "signaled exit with zero code", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.Events[0].Request.ExitEvent.GuestSignal = 9 }, wantRead: true},
		{name: "oom exit with zero code", status: 409, message: abrupt, mutate: func(m *fly.Machine) { m.Events[0].Request.ExitEvent.OOMKilled = true }, wantRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := &fly.MachineEvent{ID: "start", Type: "start", Status: "started", Timestamp: 200}
			exit := &fly.MachineEvent{ID: "exit", Type: "exit", Status: "stopped", Timestamp: 300, Request: &fly.MachineRequest{
				ExitEvent: &fly.MachineExitEvent{ExitedAt: time.UnixMilli(300)},
			}}
			creation := &fly.MachineEvent{ID: "create", Type: "launch", Status: "created", Timestamp: 100}
			expected := fly.Machine{
				ID: machineID, InstanceID: "release-version", State: fly.MachineStateStopped,
				Config: &fly.MachineConfig{AutoDestroy: true, Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyProcessGroup: fly.MachineProcessGroupFlyAppReleaseCommand,
					fly.MachineConfigMetadataKeyFlyReleaseId:    "fixture-release",
					"fly_release_command_attempt":               "fixture-attempt",
				}},
				Events: []*fly.MachineEvent{creation},
			}
			fresh := expected
			fresh.Events = []*fly.MachineEvent{exit, start, creation}
			if tc.ordinaryAnchor {
				expected.Events = []*fly.MachineEvent{{ID: "pending", Type: "launch", Status: "pending", Timestamp: 75}}
				fresh.Events = []*fly.MachineEvent{exit, start, {ID: "built", Type: "update", Status: "stopped", Timestamp: 120}, creation, expected.Events[0]}
			}
			if tc.historyMode != "" && !tc.ordinaryAnchor {
				built := &fly.MachineEvent{ID: "built", Type: "update", Status: "stopped", Timestamp: 120}
				expected.Events = []*fly.MachineEvent{built, creation}
				fresh.Events = []*fly.MachineEvent{exit, start, built, creation}
			}
			full := fresh
			if tc.historyMode != "" {
				starting := &fly.MachineEvent{ID: "starting", Type: "start", Status: "starting", Timestamp: 150}
				destroying := &fly.MachineEvent{ID: "destroying", Type: "destroy", Status: "destroying", Timestamp: 400}
				destroyed := &fly.MachineEvent{ID: "destroyed", Type: "destroy", Status: "destroyed", Timestamp: 500}
				fresh.State = fly.MachineStateDestroyed
				full = fresh
				full.Events = append([]*fly.MachineEvent{destroyed, destroying, exit, start, starting}, fresh.Events[2:]...)
				fresh.Events = full.Events[:5]
				if tc.historyMode == "different prefix" {
					full.Events = append([]*fly.MachineEvent(nil), full.Events...)
					full.Events[0] = &fly.MachineEvent{ID: "different-destroy", Type: "destroy", Status: "destroyed", Timestamp: 500}
				} else if tc.historyMode == "competing action" {
					full.Events = append([]*fly.MachineEvent(nil), full.Events...)
					full.Events[5] = &fly.MachineEvent{ID: "other-stop", Type: "stop", Status: "stopped", Timestamp: 120}
				} else if tc.historyMode == "reordered creation" {
					full.Events = append([]*fly.MachineEvent(nil), full.Events...)
					full.Events[5], full.Events[6] = full.Events[6], full.Events[5]
				} else if tc.historyMode == "missing anchor" {
					full.Events = full.Events[:5]
				}
			}
			if tc.mutate != nil {
				tc.mutate(&fresh)
			}
			var calls []string
			transport := releaseStartTransport(func(r *http.Request) (*http.Response, error) {
				status := http.StatusOK
				var body any
				switch r.Method + " " + r.URL.Path {
				case "POST " + machinesPath + "/" + machineID + "/start":
					calls = append(calls, "start")
					status = tc.status
					if tc.message != "" {
						body = map[string]string{"error": tc.message}
					} else {
						body = fly.MachineStartResponse{Status: "success", PreviousState: fly.MachineStateStopped}
					}
				case "GET " + machinesPath + "/" + machineID:
					calls = append(calls, "get")
					require.Equal(t, expected.InstanceID, r.URL.Query().Get("version"))
					if tc.getStatus != 0 {
						status = tc.getStatus
						body = map[string]string{"error": "machine not found"}
					} else {
						body = fastExitWireMachine(t, fresh, tc.wireVersion, tc.omitExitCode, tc.nullExitCode, tc.monitorEvent)
					}
				case "GET " + machinesPath + "/" + machineID + "/events":
					calls = append(calls, "events")
					require.Equal(t, "20", r.URL.Query().Get("limit"))
					body = fastExitWireMachine(t, full, tc.wireVersion, tc.omitExitCode, tc.nullExitCode, tc.monitorEvent)["events"]
				case "GET " + machinesPath + "/" + machineID + "/wait":
					calls = append(calls, "wait-destroyed")
					require.Equal(t, fly.MachineStateDestroyed, r.URL.Query().Get("state"))
					status = http.StatusNotFound
					body = map[string]string{"error": "machine not found"}
				default:
					t.Fatalf("unexpected Machines API request %s %s", r.Method, r.URL)
				}
				payload, err := json.Marshal(body)
				require.NoError(t, err)
				headers := http.Header{"Fly-Request-Id": {"fixture-request"}}
				if status == http.StatusTemporaryRedirect {
					headers.Set("Location", "/unexpected-proof-redirect")
				}
				return &http.Response{
					StatusCode: status, Header: headers,
					Body: io.NopCloser(strings.NewReader(string(payload))), Request: r,
				}, nil
			})
			originalTransport := http.DefaultTransport
			http.DefaultTransport = transport
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			client, err := flaps.NewWithOptions(t.Context(), flaps.NewClientOpts{Transport: transport})
			require.NoError(t, err)
			md := machineDeployment{app: &flaps.App{Name: "test-app"}, flapsClient: client, releaseId: "fixture-release", waitTimeout: time.Second}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			_, startErr := client.Start(fly.WithoutHTTPRetries(ctx), md.app.Name, machineID, "fixture-lease")
			fastExit, err := md.finishReleaseCommandStart(ctx, &expected, startErr)
			require.Equal(t, tc.wantFastExit, fastExit)
			if tc.wantFastExit || tc.status == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, startErr)
			}
			wantCalls := []string{"start"}
			if tc.wantRead {
				wantCalls = append(wantCalls, "get")
			}
			if tc.historyMode != "" {
				wantCalls = append(wantCalls, "events")
			}
			if tc.waitForDestroy {
				ios, _, _, _ := iostreams.Test()
				ctx, cleanup := statuslogger.SingleLine(iostreams.NewContext(ctx, ios), false)
				defer cleanup(false)
				md.releaseCommandMachine = machine.NewMachineSet(client, ios, md.app.Name, []*fly.Machine{&expected}, false)
				lm := md.releaseCommandMachine.GetMachines()[0]
				require.NoError(t, md.waitForReleaseCommandDestroyed(ctx, lm))
				require.True(t, md.releaseCommandMachine.IsEmpty())
				wantCalls = append(wantCalls, "wait-destroyed")
			}
			require.Equal(t, wantCalls, calls)
		})
	}
}

// fly-go omits a zero ExitCode when encoding the fixture. Restore the exact
// wire distinction between a reported zero and a missing value.
func fastExitWireMachine(t *testing.T, machine fly.Machine, version string, omitExitCode, nullExitCode, useMonitorEvent bool) map[string]any {
	t.Helper()
	var body map[string]any
	encoded, err := json.Marshal(machine)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &body))
	if version != "" {
		body["version"] = version
	}
	for i, event := range machine.Events {
		if event.Request == nil || event.Request.ExitEvent == nil {
			continue
		}
		request := body["events"].([]any)[i].(map[string]any)["request"].(map[string]any)
		exit := request["exit_event"].(map[string]any)
		switch {
		case omitExitCode:
			delete(exit, "exit_code")
		case nullExitCode:
			exit["exit_code"] = nil
		default:
			exit["exit_code"] = event.Request.ExitEvent.ExitCode
		}
		if useMonitorEvent {
			delete(request, "exit_event")
			request["MonitorEvent"] = map[string]any{"exit_event": exit}
		}
	}
	return body
}
