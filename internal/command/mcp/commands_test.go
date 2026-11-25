package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/mock"
)

// testCommand parses args for cmd and returns a context to run it with. HOME
// points at a temporary directory, so no test can touch the MCP client
// configurations of whoever runs it.
func testCommand(t *testing.T, cmd *cobra.Command, args ...string) context.Context {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	require.NoError(t, cmd.Flags().Parse(args))
	ctx := logger.NewContext(t.Context(), logger.New(os.Stderr, logger.Error, false))
	ctx = flag.NewContext(ctx, cmd.Flags())

	return flag.WithExtraArgs(ctx, nil)
}

// recordFlyctl replaces flyctl for the test and returns the commands run.
func recordFlyctl(t *testing.T) *[][]string {
	t.Helper()

	var calls [][]string
	original := flyctl
	flyctl = func(args ...string) error {
		calls = append(calls, args)

		return nil
	}
	t.Cleanup(func() { flyctl = original })

	return &calls
}

func writeServers(t *testing.T, path, key string, servers map[string][]string) {
	t.Helper()

	entries := map[string]any{}
	for name, args := range servers {
		entries[name] = map[string]any{"command": "flyctl", "args": args}
	}
	data, err := json.Marshal(map[string]any{key: entries})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func entryArgs(t *testing.T, path, server string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed struct {
		McpServers map[string]MCPServer `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Contains(t, parsed.McpServers, server)

	return parsed.McpServers[server].Args
}

func argAfter(args []string, name string) string {
	if i := slices.Index(args, name); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}

	return ""
}

func TestAddWithoutClientLeavesSecretsAlone(t *testing.T) {
	calls := recordFlyctl(t)
	ctx := testCommand(t, NewAdd(), "--url", "https://my-mcp.fly.dev")
	ctx = appconfig.WithConfig(ctx, &appconfig.Config{AppName: "my-mcp"})

	require.ErrorContains(t, runAdd(ctx), "no configuration paths found")
	require.Empty(t, *calls)
}

func TestAddWritesGivenPassword(t *testing.T) {
	recordFlyctl(t)
	config := filepath.Join(t.TempDir(), "mcp.json")
	ctx := testCommand(t, NewAdd(), "--url", "https://my-mcp.fly.dev", "--config", config, "--user", "bob", "--password", "s3cret")
	ctx = appconfig.WithConfig(ctx, &appconfig.Config{AppName: "my-mcp"})

	require.NoError(t, runAdd(ctx))
	args := entryArgs(t, config, "my-mcp")
	require.Equal(t, "bob", argAfter(args, "--user"))
	require.Equal(t, "s3cret", argAfter(args, "--password"))
}

// releaseClient answers the query for an app's released configuration.
type releaseClient struct{}

func (releaseClient) MakeRequest(_ context.Context, _ *genq.Request, resp *genq.Response) error {
	return json.Unmarshal([]byte(`{"app":{"currentReleaseUnprocessed":{"configDefinition":{"app":"other"}}}}`), resp.Data)
}

func TestAddAppFlagWinsOverAppConfig(t *testing.T) {
	for _, tc := range []struct{ flag, want string }{
		{"other", "other"},
		{"web", "web"},
		{"", "web"},
	} {
		t.Run("app="+tc.flag, func(t *testing.T) {
			calls := recordFlyctl(t)
			config := filepath.Join(t.TempDir(), "mcp.json")
			args := []string{"--flycast", "--config", config}
			if tc.flag != "" {
				args = append(args, "--app", tc.flag)
			}
			ctx := testCommand(t, NewAdd(), args...)
			ctx = appconfig.WithConfig(ctx, &appconfig.Config{AppName: "web"})
			ctx = appconfig.WithName(ctx, tc.want)
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GenqClientFunc: func() genq.Client { return releaseClient{} }})

			require.NoError(t, runAdd(ctx))
			require.Len(t, *calls, 1)
			require.Equal(t, tc.want, argAfter((*calls)[0], "--app"))
			require.Equal(t, "http://"+tc.want+".flycast/", argAfter(entryArgs(t, config, tc.want), "--url"))
		})
	}
}

// zedPath is where ListConfigPaths looks for Zed's configuration on every OS.
func zedPath(t *testing.T) string {
	t.Helper()

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	return filepath.Join(home, ".config", "zed", "settings.json")
}

func TestSelectServerAndConfigPicksTheOnlyConfigWithServers(t *testing.T) {
	ctx := testCommand(t, NewRemove())
	writeServers(t, zedPath(t), "context_servers", map[string][]string{"my-mcp": {"mcp", "proxy"}})

	server, paths, err := SelectServerAndConfig(ctx, false)
	require.NoError(t, err)
	require.Equal(t, "my-mcp", server)
	require.Len(t, paths, 1)
	require.Equal(t, zedPath(t), paths[0].Path)
}

func TestInspectReadsCredentialsFromEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("npx is faked with a shell script")
	}

	bin := t.TempDir()
	out := filepath.Join(bin, "args")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "npx"), fmt.Appendf(nil, "#!/bin/sh\nprintf '%%s\\n' \"$@\" > '%s'\n", out), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx := testCommand(t, NewInspect(), "--zed", "--server", "my-mcp")
	writeServers(t, zedPath(t), "context_servers", map[string][]string{
		"my-mcp": {"mcp", "proxy", "--url", "https://my-mcp.fly.dev", "--user", "bob", "--password", "s3cret"},
	})

	require.NoError(t, runInspect(ctx))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	args := strings.Split(string(data), "\n")
	require.Equal(t, "https://my-mcp.fly.dev", argAfter(args, "--url"))
	require.Equal(t, "bob", argAfter(args, "--user"))
	require.Equal(t, "s3cret", argAfter(args, "--password"))

	// `fly mcp list` prints what configExtract returns.
	entry, err := configExtract(ConfigPath{Path: zedPath(t), ConfigName: "context_servers"}, "my-mcp")
	require.NoError(t, err)
	require.NotContains(t, entry, "password")
}

func pushImage(t *testing.T, config v1.Config) string {
	t.Helper()

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	ref := strings.TrimPrefix(server.URL, "http://") + "/mcp/example:latest"
	img, err := mutate.Config(empty.Image, config)
	require.NoError(t, err)
	tag, err := name.ParseReference(ref)
	require.NoError(t, err)
	require.NoError(t, remote.Write(tag, img))

	return ref
}

func TestLaunchImageWithoutCmd(t *testing.T) {
	calls := recordFlyctl(t)
	t.Chdir(t.TempDir()) // runLaunch changes directory; restore it afterwards
	ref := pushImage(t, v1.Config{Entrypoint: []string{"node", "dist/index.js"}})
	ctx := testCommand(t, NewLaunch(), "--image", ref)

	require.NoError(t, runLaunch(ctx))
	require.Len(t, *calls, 2)
	require.Equal(t, "launch", (*calls)[0][0])
	require.Equal(t, "deploy", (*calls)[1][0])
}

func TestLaunchResolvesRelativePaths(t *testing.T) {
	calls := recordFlyctl(t)
	t.Chdir(t.TempDir())
	wd, err := os.Getwd()
	require.NoError(t, err)
	ctx := testCommand(t, NewLaunch(), "--config", "mcp.json", "--file-local", "/etc/a=local.txt", "--file-local", "/etc/b=", "npx server")

	require.NoError(t, runLaunch(ctx))
	require.Len(t, *calls, 3)
	require.Equal(t, filepath.Join(wd, "mcp.json"), argAfter((*calls)[1], "--config"))
	require.Equal(t, "/etc/a="+filepath.Join(wd, "local.txt"), argAfter((*calls)[2], "--file-local"))
	require.Contains(t, (*calls)[2], "/etc/b=")
}
