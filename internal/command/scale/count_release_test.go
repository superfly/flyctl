package scale

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestScaleCountReleaseFallback(t *testing.T) {
	const valid = `{"data":{"app":{"currentReleaseUnprocessed":{"configDefinition":{"processes":{"replicas":"run"}}}}}}`
	const missing = `{"data":{"app":{}}}`
	type reply struct {
		status int
		body   string
	}
	success := reply{200, valid}
	noConfig := reply{200, missing}
	failure := reply{500, "{}"}
	unavailable := reply{503, "{}"}
	for _, tc := range []struct {
		name, group string
		replies     []reply
		wantErr     string
		empty       bool
	}{
		{"temporary failure recovers", "replicas", []reply{failure, success}, "", false},
		{"older app uses reconstructed group", "app", []reply{noConfig}, "", false},
		{"older app cannot verify named group", "replicas", []reply{noConfig}, "could not verify", false},
		{"release confirms unknown group", "worker", []reply{success}, "unknown process groups", false},
		{"retry confirms unknown group", "worker", []reply{failure, success}, "unknown process groups", false},
		{"permission failure is not retried", "replicas", []reply{{403, "{}"}}, "HTTP 403", false},
		{"malformed definition is not retried", "replicas", []reply{{200, `{"data":{"app":{"currentReleaseUnprocessed":{"configDefinition":"invalid"}}}}`}}, "could not verify", false},
		{"temporary failure with usable reconstruction", "app", []reply{failure}, "", false},
		{"retry still missing", "replicas", []reply{failure, noConfig}, "could not verify", false},
		{"empty app scales from recovered release", "replicas", []reply{failure, success}, "", true},
		{"empty app exhausts transport retries", "replicas", []reply{unavailable, unavailable, unavailable, unavailable, unavailable}, "HTTP 503", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streams, _, out, stderr := iostreams.Test()
			ctx := appconfig.WithName(iostreams.NewContext(context.Background(), streams), "synthetic")
			ctx = config.NewContext(ctx, &config.Config{})
			fs := newScaleCount().Flags()
			require.NoError(t, fs.Parse([]string{tc.group + "=1", "--region", "iad", "--yes"}))
			ctx = flag.NewContext(ctx, fs)
			requests := 0
			transport := releaseTransport(func(r *http.Request) (*http.Response, error) {
				require.Less(t, requests, len(tc.replies))
				reply := tc.replies[requests]
				requests++

				return &http.Response{StatusCode: reply.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(reply.body)), Request: r}, nil
			})
			client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: "https://example.invalid", Transport: &fly.Transport{UnderlyingTransport: transport}})
			ctx = flyutil.NewContextWithClient(ctx, client)
			ms := []*fly.Machine{{ID: "synthetic", State: "started", Region: "iad", Config: &fly.MachineConfig{Metadata: map[string]string{"fly_process_group": tc.group}}}}
			if tc.empty {
				ms = nil
			}
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListFunc: func(context.Context, string, string) ([]*fly.Machine, error) {
					if tc.empty {
						return nil, nil
					}
					// Different Machine configs produce a reconstruction warning.
					other := &fly.Machine{ID: "other", State: "started", Config: &fly.MachineConfig{Env: map[string]string{"OTHER": "yes"}}}

					return append(ms, other), nil
				},
				ListFlyAppsMachinesFunc: func(context.Context, string) ([]*fly.Machine, *fly.Machine, error) {
					require.Empty(t, tc.wantErr)
					return ms, nil, nil
				},
				GetIPAssignmentsFunc: func(context.Context, string) (*flaps.ListIPAssignmentsResponse, error) {
					return &flaps.ListIPAssignmentsResponse{}, nil
				},
				GetAppFunc: func(context.Context, string) (*flaps.App, error) { return &flaps.App{}, nil },
				LaunchFunc: func(_ context.Context, _ string, input fly.LaunchMachineInput) (*fly.Machine, error) {
					require.True(t, tc.empty)
					require.Equal(t, "replicas", input.Config.Metadata["fly_process_group"])
					require.Equal(t, []string{"run"}, input.Config.Init.Cmd)
					return &fly.Machine{ID: "created", Config: input.Config}, nil
				},
				GetVolumesFunc: func(context.Context, string) ([]fly.Volume, error) { return nil, nil },
			})
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{ListReleasesFunc: func(context.Context, string, int) ([]uiex.Release, error) {
				return []uiex.Release{{Status: "complete"}}, nil
			}})
			err := runScaleCount(ctx)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else if tc.empty {
				require.NoError(t, err)
				require.Contains(t, out.String(), "Created created group:replicas")
			} else {
				require.NoError(t, err)
				require.Contains(t, out.String(), "already scaled")
			}
			if tc.group == "app" {
				require.Contains(t, stderr.String(), "WARNING [fly.toml]")
			} else {
				require.NotContains(t, stderr.String(), "WARNING [fly.toml]")
			}
			require.Equal(t, len(tc.replies), requests)
		})
	}
}
