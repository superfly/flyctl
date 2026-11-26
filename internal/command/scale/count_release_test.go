package scale

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
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
	for _, tc := range []struct {
		name, group string
		status      int
		body        string
		retryBody   string
		wantErr     string
	}{
		{"temporary failure recovers", "replicas", 503, "{}", valid, ""},
		{"older app uses reconstructed group", "app", 200, missing, "", ""},
		{"older app cannot verify named group", "replicas", 200, missing, "", "could not verify"},
		{"release confirms unknown group", "worker", 200, valid, "", "unknown process groups"},
		{"retry confirms unknown group", "worker", 503, "{}", valid, "unknown process groups"},
		{"permission failure is not retried", "replicas", 403, "{}", "", "could not verify"},
		{"malformed definition is not retried", "replicas", 200, `{"data":{"app":{"currentReleaseUnprocessed":{"configDefinition":"invalid"}}}}`, "", "could not verify"},
		{"temporary failure with usable reconstruction", "app", 503, "{}", "", ""},
		{"retry still missing", "replicas", 503, "{}", missing, "could not verify"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.Test()
			ctx := appconfig.WithName(iostreams.NewContext(context.Background(), streams), "synthetic")
			fs := newScaleCount().Flags()
			require.NoError(t, fs.Parse([]string{tc.group + "=1", "--region", "iad"}))
			ctx = flag.NewContext(ctx, fs)
			requests := 0
			transport := releaseTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				status, body := tc.status, tc.body
				if requests > 1 {
					require.Equal(t, 2, requests)
					require.NotEmpty(t, tc.retryBody)
					status, body = 200, tc.retryBody
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: "https://example.invalid", Transport: &fly.Transport{UnderlyingTransport: transport}})
			ctx = fly.WithoutHTTPRetries(flyutil.NewContextWithClient(ctx, client))
			ms := []*fly.Machine{{ID: "synthetic", State: "started", Region: "iad", Config: &fly.MachineConfig{Metadata: map[string]string{"fly_process_group": tc.group}}}}
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListFunc: func(context.Context, string, string) ([]*fly.Machine, error) { return ms, nil },
				ListFlyAppsMachinesFunc: func(context.Context, string) ([]*fly.Machine, *fly.Machine, error) {
					require.Empty(t, tc.wantErr)
					return ms, nil, nil
				},
				GetVolumesFunc: func(context.Context, string) ([]fly.Volume, error) { return nil, nil },
			})
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{ListReleasesFunc: func(context.Context, string, int) ([]uiex.Release, error) {
				return []uiex.Release{{Status: "complete"}}, nil
			}})
			err := runScaleCount(ctx)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, out.String(), "already scaled")
			}
			wantCalls := 1
			if tc.retryBody != "" {
				wantCalls++
			}
			require.Equal(t, wantCalls, requests)
		})
	}
}
