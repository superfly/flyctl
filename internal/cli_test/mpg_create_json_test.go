package cli_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
)

func TestMPGCreateJSONCLI(t *testing.T) {
	for _, tc := range []struct {
		stage, status string
		code          int
	}{
		{"poll", "creating", 1},
		{"failed", "failed", 1},
		{"cancel", "creating", 127},
		{"credentials", "ready", 0},
		{"endpoint", "ready", 0},
		{"env", "ready", 0},
		{"trace", "ready", 0},
		{"trace-env", "ready", 0},
		{"debug", "ready", 0},
		{"debug-env", "ready", 0},
		{"debug-text-reject", "", 1},
		{"debug-text-env-reject", "", 1},
		{"gha-poll", "creating", 1},
		{"gha-failed", "failed", 1},
		{"gha-env-poll", "creating", 1},
		{"gha-reject", "", 1},
		{"reject", "", 1},
		{"gha-text-reject", "", 1},
		{"gha-text-env-reject", "", 1},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			// CLI configuration and telemetry have process-wide state.
			annotations := strings.HasPrefix(tc.stage, "gha-")
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMPGCreateJSONProcess$")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "FLYCTL_MPG_CREATE_JSON_TEST="+tc.stage,
				fmt.Sprintf("GITHUB_ACTIONS=%t", annotations), fmt.Sprintf("FLY_GHA_ERROR_ANNOTATION=%t", annotations))
			diagnostics := strings.HasPrefix(tc.stage, "debug")
			if diagnostics {
				cmd.Env = append(cmd.Env, "LOG_LEVEL=debug", "FLYCTL_OUTPUT_HAR="+filepath.Join(cmd.Dir, "missing", "requests.har"))
			}
			var stdout, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "%s", stderr.String())
				code = exit.ExitCode()
			}
			require.Equal(t, tc.code, code, "%s", stderr.String())
			if strings.HasPrefix(tc.stage, "trace") {
				require.Contains(t, stderr.String(), `"Name": "cmd.mpg.create"`)
			}
			annotation := "::error title=flyctl error::"
			text := strings.Contains(tc.stage, "text-")
			if text && diagnostics {
				require.Contains(t, stdout.String(), "error writing HAR:")
				require.NotContains(t, stderr.String(), "error writing HAR:")
			} else if text && annotations {
				require.Equal(t, annotation+"failed creating managed postgres cluster: create rejected\n", stdout.String())
				require.NotContains(t, stderr.String(), annotation)
			} else if tc.status == "" {
				require.Empty(t, stdout.String())
			} else {
				require.JSONEq(t, `{"id":"new-id","name":"my-db","organization":{"Name":"","Slug":"my-org"},"region":"iad","plan":"basic","status":"`+tc.status+`"}`, stdout.String())
			}
			if diagnostics && !text {
				require.Contains(t, stderr.String(), "DEBUG")
				require.Contains(t, stderr.String(), "error writing HAR:")
			}
			if annotations && !text && tc.code == 1 {
				require.Contains(t, stderr.String(), annotation)
			}
			if tc.status == "" {
				require.Contains(t, stderr.String(), "Error: failed creating managed postgres cluster: create rejected")
			} else if tc.code != 0 {
				require.Contains(t, stderr.String(), "Error: cluster new-id was created, but waiting for readiness failed:")
			}
			t.Logf("exit=%d stdout=%s", code, stdout.String())
		})
	}
}

// Block HTTP through the default transport. TestMain separately gives the trace
// exporter, which owns its HTTP client, a collector URL that fails URL parsing,
// so nothing is dialed. All command API calls use mocks.
type mpgNoNetwork struct{}

func (mpgNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network disabled in MPG CLI test")
}

func TestMPGCreateJSONProcess(t *testing.T) {
	stage := os.Getenv("FLYCTL_MPG_CREATE_JSON_TEST")
	if stage == "" {
		t.Skip("subprocess helper")
	}
	stage = strings.TrimPrefix(stage, "gha-")
	stage = strings.TrimPrefix(strings.TrimPrefix(stage, "debug"), "-")
	text := strings.HasPrefix(stage, "text-")
	stage = strings.TrimPrefix(stage, "text-")
	if strings.HasPrefix(stage, "trace") {
		t.Setenv("LOG_LEVEL", "trace")
		stage = strings.TrimPrefix(strings.TrimPrefix(stage, "trace"), "-")
	}
	fromEnv := stage == "env" || strings.HasPrefix(stage, "env-")
	stage = strings.TrimPrefix(stage, "env-")
	http.DefaultTransport = mpgNoNetwork{}
	perm, err := macaroon.New([]byte("fixture"), flyio.LocationPermission, macaroon.NewSigningKey())
	require.NoError(t, err)
	token, err := perm.String()
	require.NoError(t, err)
	for key, value := range map[string]string{
		"FLY_CONFIG_DIR":   filepath.Join(t.TempDir(), "config"),
		"FLY_ACCESS_TOKEN": token, "FLY_API_TOKEN": "", "FLY_APP": "",
		"FLY_ORG": "", "FLY_ORGANIZATION": "", "FLY_JSON": "0",
		"FLY_NO_UPDATE_CHECK": "1", "FLY_UPDATE_CHECK": "0",
		"FLY_NO_INCIDENTS_CHECK": "1", "FLY_INCIDENTS_CHECK": "0",
		"FLY_SEND_METRICS": "0", "FLY_NO_PROMPT": "1", "NO_COLOR": "1",
	} {
		t.Setenv(key, value)
	}
	ctx, cancel := context.WithCancel(metrics.WithDisableFlushMetrics(t.Context()))
	defer cancel()
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{AuthenticatedFunc: func() bool { return true }})
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		ListOrganizationsFunc: func(context.Context, bool) ([]uiex.Organization, error) {
			return []uiex.Organization{{Slug: "my-org", RawSlug: "my-org"}}, nil
		},
	})
	credentials := 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetRegionsFunc: func(context.Context) (*flaps.RegionData, error) {
			return &flaps.RegionData{Regions: []fly.Region{{Code: "iad", MPGAvailable: true}}}, nil
		},
		CreateManagedPostgresClusterFunc: func(context.Context, flaps.CreateManagedPostgresClusterRequest) (flaps.ManagedPostgresCluster, error) {
			if stage == "reject" {
				return flaps.ManagedPostgresCluster{}, errors.New("create rejected")
			}
			return flaps.ManagedPostgresCluster{ID: "new-id", Region: "iad", Status: "creating"}, nil
		},
		GetManagedPostgresClusterFunc: func(ctx context.Context, _ string) (flaps.ManagedPostgresCluster, error) {
			switch stage {
			case "poll":
				return flaps.ManagedPostgresCluster{}, errors.New("poll failed")
			case "cancel":
				cancel()
				return flaps.ManagedPostgresCluster{}, ctx.Err()
			case "failed":
				return flaps.ManagedPostgresCluster{ID: "new-id", Status: "failed"}, nil
			}
			ready := flaps.ManagedPostgresCluster{ID: "new-id", Status: "ready"}
			if stage != "endpoint" {
				ready.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.example.test", Port: 6432}
			}

			return ready, nil
		},
		GetManagedPostgresUserCredentialsFunc: func(context.Context, string, string) (flaps.ManagedPostgresUserCredentials, error) {
			credentials++
			if stage == "endpoint" || stage == "env" {
				return flaps.ManagedPostgresUserCredentials{Password: "test-password"}, nil
			}
			return flaps.ManagedPostgresUserCredentials{}, errors.New("credentials failed")
		},
	})
	args := []string{"mpg", "create", "--name", "my-db", "--org", "my-org", "--region", "iad"}
	if fromEnv {
		t.Setenv("FLY_JSON", "1")
	}
	if text && fromEnv {
		args = append(args, "--json=false")
	} else if !text && !fromEnv {
		args = append(args, "--json")
	}
	stdout, stderr, code := capture(ctx, t, args...)
	require.Zero(t, credentials)
	// In JSON mode cli.Run leaves os.Stdout on stderr; write to the real one.
	_, _ = io.WriteString(iostreams.Stdout, stdout)
	_, _ = io.WriteString(os.Stderr, stderr)
	os.Exit(code)
}
