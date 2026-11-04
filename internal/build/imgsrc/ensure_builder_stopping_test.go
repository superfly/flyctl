package imgsrc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

type builderTransport func(*http.Request) (*http.Response, error)

func (f builderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEnsureStoppingBuilder(t *testing.T) {
	t.Setenv("FLY_FLAPS_BASE_URL", "http://builder.invalid")
	type step struct {
		method, path, body string
		status             int
	}
	response := func(method, path, body string) step { return step{method, path, body, 200} }
	failure := func(method, path string, code int, message string) step {
		body, err := json.Marshal(map[string]string{"error": message})
		if err != nil {
			t.Fatal(err)
		}
		return step{method, path, string(body), code}
	}
	wait := response("GET", "/proof/wait?state=stopped&state=started&timeout=60", `{}`)
	start := response("POST", "/proof/start", `{}`)
	ready := response("GET", "/proof/wait?state=started&timeout=60", `{}`)
	stopping := failure("POST", "/proof/start", 412, "machine still stopping, refusing to start")
	type testCase struct {
		name    string
		steps   []step
		want    string
		elapsed time.Duration
		replace bool
		direct  bool
	}
	tests := []testCase{
		{name: "stop completes", steps: []step{wait, start, ready}},
		{name: "other caller already started", steps: []step{wait, response("POST", "/proof/start", `{"previous_state":"started"}`), ready}},
		{name: "other caller starts after wait", steps: []step{wait, failure("POST", "/proof/start", 409, "machine still attempting to start"), ready}},
		{name: "stopped before cleanup completes", steps: []step{wait, stopping, stopping, start, ready}, elapsed: 2 * time.Second},
		{name: "stops again before start", steps: []step{wait, failure("POST", "/proof/start", 412, "unable to start machine from current state: 'stopping'"), start, ready}, elapsed: time.Second},
		{name: "lease conflict", steps: []step{wait, failure("POST", "/proof/start", 409, "machine lease is held by another caller")}, want: "machine lease"},
		{name: "unknown precondition", steps: []step{wait, failure("POST", "/proof/start", 412, "machine getting replaced, refusing to start")}, want: "machine getting replaced"},
		{name: "wrong status for stopping message", steps: []step{wait, failure("POST", "/proof/start", 403, "machine still stopping, refusing to start")}, want: "machine still stopping"},
		{name: "wrong status for starting message", steps: []step{wait, failure("POST", "/proof/start", 412, "machine still attempting to start")}, want: "machine still attempting"},
		{name: "resource reservation still requests replacement", steps: []step{wait, failure("POST", "/proof/start", 412, "could not reserve resource for machine")}, replace: true, direct: true},
		{name: "disabled host still requests replacement", steps: []step{wait, failure("POST", "/proof/start", 412, "deploys to this host are temporarily disabled")}, replace: true, direct: true},
		{name: "stop wait cannot request replacement", steps: []step{failure("GET", wait.path, 412, "could not reserve resource for machine")}, want: "could not reserve"},
		{name: "started wait cannot request replacement", steps: []step{wait, start, failure("GET", ready.path, 412, "could not reserve resource for machine")}, want: "could not reserve"},
	}
	for _, code := range []int{403, 404, 408, 500} {
		tests = append(tests, testCase{name: fmt.Sprintf("wait error %d", code), steps: []step{failure("GET", wait.path, code, "wait rejected")}, want: "wait rejected"})
		tests = append(tests, testCase{name: fmt.Sprintf("start error %d", code), steps: []step{wait, failure("POST", start.path, code, "start rejected")}, want: "start rejected"})
		tests = append(tests, testCase{name: fmt.Sprintf("readiness error %d", code), steps: []step{wait, start, failure("GET", ready.path, code, "readiness rejected")}, want: "readiness rejected"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				next := 0
				ctx := testingContext(t)
				fc, err := flaps.NewWithOptions(ctx, flaps.NewClientOpts{Transport: builderTransport(func(r *http.Request) (*http.Response, error) {
					path := r.URL.Path
					var s step
					switch {
					case !tt.direct && r.Method == "GET" && path == "/v1/apps":
						s = response("GET", path, `{"apps":[{"name":"builder"}]}`)
					case !tt.direct && r.Method == "GET" && path == "/v1/apps/builder/machines":
						s = response("GET", path, `[{"id":"proof","state":"stopping"}]`)
					default:
						if next == len(tt.steps) {
							return nil, fmt.Errorf("unexpected operation %s %s", r.Method, r.URL.RequestURI())
						}
						s = tt.steps[next]
						next++
						got := strings.TrimPrefix(r.URL.RequestURI(), "/v1/apps/builder/machines")
						if r.Method != s.method || got != s.path {
							t.Errorf("operation %d: got %s %s; want %s %s", next, r.Method, got, s.method, s.path)
							return nil, errors.New("unexpected API operation")
						}
					}

					return &http.Response{StatusCode: s.status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(s.body)), Request: r}, nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				ctx = flapsutil.NewContextWithClient(ctx, fc)
				begin := time.Now()
				if tt.direct {
					err = startOrRestartBuilderMachine(ctx, "builder", &fly.Machine{ID: "proof", State: "stopping"})
				} else {
					_, _, err = (&Provisioner{orgSlug: "org"}).EnsureBuilder(ctx, "iad", false)
				}
				if tt.replace {
					if !errors.Is(err, ShouldReplaceBuilderMachine) {
						t.Fatalf("want resource replacement, got %v", err)
					}
				} else if tt.want != "" {
					if err == nil || !strings.Contains(err.Error(), tt.want) || errors.Is(err, ShouldReplaceBuilderMachine) {
						t.Fatalf("want %q without replacement, got %v", tt.want, err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if next != len(tt.steps) {
					t.Fatalf("consumed %d/%d API operations", next, len(tt.steps))
				}
				// Error responses must not silently become backoff retries (fly-go may
				// independently retry transport failures; these scripted HTTP responses do not).
				if elapsed := time.Since(begin); elapsed != tt.elapsed {
					t.Fatalf("elapsed %s, want %s", elapsed, tt.elapsed)
				}
			})
		})
	}
}

func TestStoppingBuilderRecoveryDeadline(t *testing.T) {
	for _, scenario := range []string{"stop wait", "cleanup retries", "shared budget", "cancel during delay", "cancel after stop wait"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				starts, waits := 0, 0
				c := &mock.FlapsClient{
					WaitFunc: func(ctx context.Context, _, _ string, _ ...flaps.WaitOption) error {
						waits++
						if scenario == "stop wait" || waits == 2 {
							<-ctx.Done()
							return ctx.Err()
						}
						if scenario == "shared budget" {
							time.Sleep(140 * time.Second)
						}
						if scenario == "cancel after stop wait" {
							cancel()
						}

						return nil
					},
					StartFunc: func(ctx context.Context, _, _, _ string) (*fly.MachineStartResponse, error) {
						starts++
						if scenario == "shared budget" {
							time.Sleep(30 * time.Second)
							return nil, nil
						}
						if scenario == "cancel during delay" {
							time.AfterFunc(500*time.Millisecond, cancel)
						}

						return nil, &flaps.FlapsError{ResponseStatusCode: 412, OriginalError: errors.New("machine still stopping, refusing to start")}
					},
				}
				begin := time.Now()
				ctx = flapsutil.NewContextWithClient(ctx, c)
				err := startOrRestartBuilderMachine(ctx, "builder", &fly.Machine{ID: "proof", State: "stopping"})
				wantErr, wantElapsed := context.DeadlineExceeded, 180*time.Second
				if strings.HasPrefix(scenario, "cancel") {
					wantErr = context.Canceled
					wantElapsed = 0
					if scenario == "cancel during delay" {
						wantElapsed = 500 * time.Millisecond
					}
				}
				if !errors.Is(err, wantErr) || time.Since(begin) != wantElapsed {
					t.Fatalf("elapsed=%s err=%v", time.Since(begin), err)
				}
				wantStarts := map[string]int{"stop wait": 0, "cleanup retries": 180, "shared budget": 1, "cancel during delay": 1, "cancel after stop wait": 0}[scenario]
				if (scenario == "cleanup retries" && (starts < 180 || starts > 181)) || (scenario != "cleanup retries" && starts != wantStarts) {
					t.Fatalf("starts=%d, want %d", starts, wantStarts)
				}
			})
		})
	}
}

func TestStoppingBuilderDoesNotRetryTransportErrors(t *testing.T) {
	for _, startErr := range []error{io.ErrUnexpectedEOF, errors.New("machine still stopping, refusing to start")} {
		t.Run(startErr.Error(), func(t *testing.T) {
			starts := 0
			c := &mock.FlapsClient{
				WaitFunc: func(context.Context, string, string, ...flaps.WaitOption) error { return nil },
				StartFunc: func(context.Context, string, string, string) (*fly.MachineStartResponse, error) {
					starts++
					return nil, startErr
				},
			}
			ctx := flapsutil.NewContextWithClient(context.Background(), c)
			err := startOrRestartBuilderMachine(ctx, "builder", &fly.Machine{ID: "proof", State: "stopping"})
			if !errors.Is(err, startErr) || starts != 1 {
				t.Fatalf("starts=%d err=%v", starts, err)
			}
		})
	}
}
