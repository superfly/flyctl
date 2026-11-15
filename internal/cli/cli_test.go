package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/iostreams"
)

func TestFlyNoPrompt(t *testing.T) {
	for val, interactive := range map[string]bool{"": true, "1": false} {
		t.Setenv("FLY_NO_PROMPT", val)

		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetStdoutTTY(true)
		setNeverPromptFromEnv(ios)

		assert.Equal(t, interactive, ios.IsInteractive(), "FLY_NO_PROMPT=%q", val)
	}
}

func TestUnknownGroupSubcommand(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	ios, _, stdout, stderr := iostreams.Test()
	code := Run(context.Background(), ios, "mpg", "lst")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, strings.ReplaceAll("Error: unknown command \"lst\" for \"fly mpg\"\n\nDid you mean this?\n\tlist\n\nRun 'fly mpg --help' for usage.\n\n", "fly", filepath.Base(executable)), stderr.String())
}

// spritesExecAPI is the least of the Sprites API that `fly sprites exec`
// needs: a WebSocket that reports the given output and exit status.
func spritesExecAPI(t *testing.T, stdout string, status byte) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sprite-token" || !websocket.IsWebSocketUpgrade(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck

		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{1}, stdout...))
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{3, status})
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

		// Hold the connection until the client has read the exit.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(api.Close)

	return api
}

// runCLI runs flyctl with args in a child process and returns its exit
// status and output. Run records process-wide metrics, so it can only run one
// command per process.
func runCLI(t *testing.T, env []string, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIChild$")
	encoded, err := json.Marshal(args)
	require.NoError(t, err)

	cmd.Env = append(os.Environ(), "FLY_CLI_TEST_CHILD="+string(encoded))
	cmd.Env = append(cmd.Env, env...)

	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	err = cmd.Run()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("failed to run the child process: %v", err)
	}

	return cmd.ProcessState.ExitCode(), out.String(), errOut.String()
}

// TestCLIChild is the child process of runCLI, not a test of its own.
func TestCLIChild(t *testing.T) {
	args, ok := os.LookupEnv("FLY_CLI_TEST_CHILD")
	if !ok {
		t.Skip("only runs as a child of runCLI")
	}

	var argv []string
	require.NoError(t, json.Unmarshal([]byte(args), &argv))

	os.Exit(Run(context.Background(), iostreams.System(), argv...))
}

func spritesEnv(t *testing.T, apiURL, token string) []string {
	t.Helper()

	return []string{
		"FLY_CONFIG_DIR=" + t.TempDir(),
		"SPRITE_TOKEN=" + token,
		"SPRITES_API_URL=" + apiURL,
		"FLY_ACCESS_TOKEN=",
		"FLY_API_TOKEN=",
		"FLY_NO_UPDATE_CHECK=1",
	}
}

// The whole path from the command line to the process exit status: a remote
// command's status becomes flyctl's, and nothing is printed about it.
func TestSpritesExecRelaysExitStatus(t *testing.T) {
	for _, status := range []byte{0, 7} {
		api := spritesExecAPI(t, "remote output\n", status)

		code, stdout, stderr := runCLI(t, spritesEnv(t, api.URL, "sprite-token"),
			"sprites", "exec", "--sprite", "dev", "--no-stdin", "--", "sh", "-c", "exit 7")

		assert.Equal(t, int(status), code)
		assert.Equal(t, "remote output\n", stdout)
		assert.NotContains(t, stderr, "Error", "a relayed exit status is not a flyctl error")
	}
}

func TestSpritesCommandFailurePrintsError(t *testing.T) {
	api := spritesExecAPI(t, "", 0)

	// A real failure, unlike a relayed status, is reported and exits 1.
	code, _, stderr := runCLI(t, spritesEnv(t, api.URL, "wrong-token"),
		"sprites", "exec", "--sprite", "dev", "--no-stdin", "true")

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "Error: failed to start command")
}

func TestSpritesAliasAndHelp(t *testing.T) {
	// `fly sprite` is the same group, and its help lists the commands.
	code, stdout, _ := runCLI(t, []string{"FLY_CONFIG_DIR=" + t.TempDir()}, "sprite", "--help")

	assert.Equal(t, 0, code)

	for _, name := range []string{"create", "exec", "console", "checkpoint", "proxy", "file"} {
		assert.Contains(t, stdout, "\n  "+name+" ")
	}
}
