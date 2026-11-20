package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

// Scripts pipe `fly launch --manifest` into jq. Scanners print to stdout three
// ways: iostreams, fmt.Printf (Django) and the terminal logger (Flask).
func TestLaunchManifestStdoutIsOnlyJSON(t *testing.T) {
	flaskApp := map[string]string{
		"requirements.txt": "flask\ngunicorn\n",
		"app.py":           "from flask import Flask\napp = Flask(__name__)\n",
	}

	for _, tc := range []struct {
		name   string
		files  map[string]string
		stderr []string
	}{
		{"django", djangoApp, []string{"Scanning source code", "WARNING: WSGI entrypoint detected"}},
		{"flask", flaskApp, []string{"Scanning source code", "Detected requirements.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my-app")
			writeFiles(t, dir, tc.files)
			t.Chdir(dir)

			ctx, _, _ := newManifestEnv().launchContext(t, dir, "--manifest")
			ctx = config.NewContext(ctx, &config.Config{Tokens: tokens.Parse("test-token")})
			t.Setenv("FLY_TRACE_COLLECTOR_URL", "127.0.0.1")

			stdout, stderr := captureProcessOutput(t)
			ios := iostreams.FromContext(ctx)
			ios.Out, ios.ErrOut = stdout, stderr
			ctx = logger.NewContext(ctx, logger.New(stderr, logger.Info, false))

			require.NoError(t, run(ctx))

			out := readFile(t, stdout)
			require.True(t, json.Valid(out), "stdout is not JSON:\n%s", out)
			var m LaunchManifest
			require.NoError(t, json.Unmarshal(out, &m))
			assert.Equal(t, "my-app", m.Plan.AppName)

			errOut := string(readFile(t, stderr))
			for _, s := range tc.stderr {
				assert.Contains(t, errOut, s)
			}
		})
	}
}

// captureProcessOutput points os.Stdout, os.Stderr and the terminal logger,
// which writes to the process's stdout, at files the test reads.
func captureProcessOutput(t *testing.T) (stdout, stderr *os.File) {
	t.Helper()

	var err error
	stdout, err = os.Create(filepath.Join(t.TempDir(), "stdout"))
	require.NoError(t, err)
	stderr, err = os.Create(filepath.Join(t.TempDir(), "stderr"))
	require.NoError(t, err)

	origStdout, origStderr, origLogger := os.Stdout, os.Stderr, terminal.DefaultLogger
	os.Stdout, os.Stderr = stdout, stderr
	terminal.DefaultLogger = logger.New(stdout, logger.Info, false)
	t.Cleanup(func() {
		os.Stdout, os.Stderr, terminal.DefaultLogger = origStdout, origStderr, origLogger
		stdout.Close()
		stderr.Close()
	})

	return stdout, stderr
}

func readFile(t *testing.T, f *os.File) []byte {
	t.Helper()

	b, err := os.ReadFile(f.Name())
	require.NoError(t, err)

	return b
}
