package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

type criticalCommandCase struct {
	Name                         string
	Args                         []string
	Mode, App, EnvApp, ConfigApp string
	Code                         int
	Output, Error                string
}

func TestCriticalCommandsCLI(t *testing.T) {
	cases := []criticalCommandCase{
		{Name: "revoke succeeds", Args: []string{"tokens", "revoke", "one", "two"}, Mode: "revoke", Output: "2 tokens revoked"},
		{Name: "revoke all fail", Args: []string{"tokens", "revoke", "one", "two"}, Mode: "revoke-all-fail", Code: 1, Error: "failed to revoke 2 of 2 tokens"},
		{Name: "revoke partially fails", Args: []string{"tokens", "revoke", "one", "two"}, Mode: "revoke-partial", Code: 1, Output: "1 tokens revoked", Error: "failed to revoke 1 of 2 tokens"},
		{Name: "volume flag overrides env and config", Args: []string{"vol", "rm", "vol_one", "--yes", "--app", "flag-app"}, Mode: "volume", App: "flag-app", EnvApp: "env-app", ConfigApp: "config-app", Output: "Destroyed volume ID: vol_one"},
		{Name: "volume env overrides config", Args: []string{"volume", "destroy", "vol_one", "-y"}, Mode: "volume", App: "env-app", EnvApp: "env-app", ConfigApp: "config-app", Output: "Destroyed volume ID: vol_one"},
		{Name: "volume uses config", Args: []string{"volumes", "destroy", "vol_one", "-y"}, Mode: "volume", App: "config-app", ConfigApp: "config-app", Output: "Destroyed volume ID: vol_one"},
		{Name: "volume infers app", Args: []string{"volumes", "destroy", "vol_one", "-y"}, Mode: "volume-infer", App: "inferred-app", Output: "Destroyed volume ID: vol_one"},
		{Name: "secrets stage flag reaches handler", Args: []string{"secrets", "set", "PASSWORD=private-value", "--stage", "--app", "flag-app"}, Mode: "secrets", App: "flag-app", EnvApp: "env-app", ConfigApp: "config-app", Output: "Secrets have been staged"},
		{Name: "deploy token flags reach API", Args: []string{"tokens", "create", "deploy", "--app", "flag-app", "--expiry", "30m", "--name", "CI job", "--json"}, Mode: "create", App: "flag-app", EnvApp: "env-app", ConfigApp: "config-app", Output: "issued-token"},
		{Name: "apps require confirmation", Args: []string{"apps", "destroy", "production"}, Code: 1, Error: "yes flag must be specified"},
		{Name: "apps require names", Args: []string{"apps", "destroy", "--yes"}, Code: 1, Error: "no app names provided"},
		{Name: "volumes require target", Args: []string{"volumes", "destroy", "--yes"}, Code: 1, Error: "volume ID or app required"},
		{Name: "tokens require IDs", Args: []string{"tokens", "revoke"}, Code: 1, Error: "no token IDs"},
		{Name: "set requires arguments", Args: []string{"secrets", "set"}, Code: 1, Error: "requires at least 1 arg"},
		{Name: "unset requires arguments", Args: []string{"secrets", "unset"}, Code: 1, Error: "requires at least 1 arg"},
		{Name: "invalid expiry rejected", Args: []string{"tokens", "create", "deploy", "--expiry", "tomorrow"}, Code: 1, Error: "invalid duration"},
		{Name: "unknown flag rejected", Args: []string{"volumes", "destroy", "--definitely-invalid"}, Code: 1, Error: "unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			// Metrics and client initialization keep process-wide state. Run a
			// fresh CLI process, as users do, instead of resetting those globals.
			data, err := json.Marshal(tc)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCriticalCommandProcess$")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "FLYCTL_CRITICAL_COMMAND_TEST="+string(data))
			output, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "%s", output)
				code = exit.ExitCode()
			}
			require.Contains(t, string(output), "CLI_CHECKS_PASSED", "helper assertions failed: %s", output)
			require.Equal(t, tc.Code, code, "%s", output)
			if tc.Output != "" {
				require.Contains(t, string(output), tc.Output)
			}
			if tc.Error != "" {
				require.Contains(t, string(output), tc.Error)
			}
		})
	}
}

func TestCriticalCommandProcess(t *testing.T) {
	data := os.Getenv("FLYCTL_CRITICAL_COMMAND_TEST")
	if data == "" {
		t.Skip("subprocess helper")
	}
	var tc criticalCommandCase
	require.NoError(t, json.Unmarshal([]byte(data), &tc))
	dir, err := os.Getwd()
	require.NoError(t, err)
	t.Setenv("FLY_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("FLY_ACCESS_TOKEN", "test-token")
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("FLY_APP", tc.EnvApp)
	t.Setenv("FLY_ORG", "")
	t.Setenv("FLY_ORGANIZATION", "")
	t.Setenv("FLY_NO_UPDATE_CHECK", "1")
	t.Setenv("FLY_UPDATE_CHECK", "0")
	t.Setenv("FLY_NO_INCIDENTS_CHECK", "1")
	t.Setenv("FLY_INCIDENTS_CHECK", "0")
	t.Setenv("FLY_SEND_METRICS", "0")
	t.Setenv("FLY_NO_PROMPT", "1")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FLY_JSON", "0")
	if tc.ConfigApp != "" {
		require.NoError(t, os.WriteFile("fly.toml", []byte("app = '"+tc.ConfigApp+"'\n"), 0600))
	}
	ctx := metrics.WithDisableFlushMetrics(t.Context())
	mutations, inferred := 0, 0
	var revoked []string
	client := &mock.Client{
		AuthenticatedFunc: func() bool { return true },
		RevokeLimitedAccessTokenFunc: func(_ context.Context, id string) error {
			revoked = append(revoked, id)
			if tc.Mode == "revoke-all-fail" || (tc.Mode == "revoke-partial" && id == "two") {
				return errors.New("permission denied")
			}
			return nil
		},
		GetAppNameFromVolumeFunc: func(_ context.Context, id string) (*string, error) {
			inferred++
			require.Equal(t, "vol_one", id)
			return fly.Pointer(tc.App), nil
		},
		GenqClientFunc: func() graphql.Client {
			return mock.GraphQLClient(func(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
				mutations++
				require.Equal(t, "CreateLimitedAccessToken", req.OpName)
				vars, err := json.Marshal(req.Variables)
				require.NoError(t, err)
				require.JSONEq(t, `{"name":"CI job","organizationId":"org-42","profile":"deploy","profileParams":{"app_id":"flag-app"},"expiry":"30m0s"}`, string(vars))

				return json.Unmarshal([]byte(`{"createLimitedAccessToken":{"limitedAccessToken":{"tokenHeader":"issued-token"}}}`), resp.Data)
			})
		},
	}
	ctx = flyutil.NewContextWithClient(ctx, client)
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
		require.Equal(t, "owner", slug)
		return &uiex.Organization{ID: "org-42"}, nil
	}})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
			require.Equal(t, tc.App, name)
			return &flaps.App{Name: name, Status: "deployed", Organization: flaps.AppOrganizationInfo{Slug: "owner"}}, nil
		},
		DeleteVolumeFunc: func(_ context.Context, app, id string) (*fly.Volume, error) {
			mutations++
			require.Equal(t, tc.App, app)
			require.Equal(t, "vol_one", id)
			return &fly.Volume{ID: id, Name: "data"}, nil
		},
		UpdateAppSecretsFunc: func(_ context.Context, app string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			mutations++
			require.Equal(t, tc.App, app)
			require.Equal(t, map[string]*string{"PASSWORD": fly.Pointer("private-value")}, values)
			return &fly.UpdateAppSecretsResp{Version: 42}, nil
		},
	})
	stdout, stderr, code := capture(ctx, t, tc.Args...)
	switch tc.Mode {
	case "revoke", "revoke-all-fail", "revoke-partial":
		require.Equal(t, []string{"one", "two"}, revoked)
	case "volume", "volume-infer", "secrets", "create":
		require.Equal(t, 1, mutations)
	default:
		require.Zero(t, mutations)
		require.Empty(t, revoked)
	}
	if tc.Mode == "volume-infer" {
		require.Equal(t, 1, inferred)
	} else {
		require.Zero(t, inferred)
	}
	if tc.Mode == "secrets" {
		require.NotContains(t, stdout+stderr, "private-value")
	}
	fmt.Fprint(os.Stdout, stdout)
	fmt.Fprint(os.Stderr, stderr)
	fmt.Fprintln(os.Stderr, "CLI_CHECKS_PASSED")
	os.Exit(code)
}
