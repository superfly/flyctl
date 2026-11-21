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
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

var flaskApp = map[string]string{
	"requirements.txt": "flask\ngunicorn\n",
	"app.py":           "from flask import Flask\napp = Flask(__name__)\n",
}

func TestLaunchManifestStdoutIsOnlyJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		files  map[string]string
		run    manifestRun
		stderr []string
	}{
		{"django", djangoApp, manifestRun{args: []string{"--manifest"}}, []string{"Scanning source code", "WARNING: WSGI entrypoint detected"}},
		{"flask", flaskApp, manifestRun{args: []string{"--manifest"}}, []string{"Scanning source code", "Detected requirements.txt"}},
		{"plan propose", flaskApp, manifestRun{propose: true}, []string{"Detected requirements.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tc.run.do(t, tc.files)

			require.True(t, json.Valid([]byte(stdout)), "stdout is not JSON:\n%s", stdout)
			var m LaunchManifest
			require.NoError(t, json.Unmarshal([]byte(stdout), &m))
			assert.Equal(t, "my-app", m.Plan.AppName)
			for _, s := range tc.stderr {
				assert.Contains(t, stderr, s)
			}
		})
	}
}

func TestLaunchManifestKeepsStdout(t *testing.T) {
	manifestFile := filepath.Join(t.TempDir(), "manifest.json")

	for _, tc := range []struct {
		name         string
		run          manifestRun
		stdout       []string
		manifestFile string
	}{
		{"manifest path", manifestRun{propose: true, args: []string{"--manifest-path", manifestFile}}, []string{"Detected requirements.txt"}, manifestFile},
		{"terminal", manifestRun{stdoutTTY: true, args: []string{"--manifest"}}, []string{"Scanning source code", "Detected requirements.txt"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _ := tc.run.do(t, flaskApp)

			for _, s := range tc.stdout {
				assert.Contains(t, stdout, s)
			}
			if tc.manifestFile != "" {
				manifest, err := os.ReadFile(tc.manifestFile)
				require.NoError(t, err)
				assert.True(t, json.Valid(manifest), "manifest file is not JSON:\n%s", manifest)
			}
		})
	}
}

type manifestRun struct {
	propose   bool
	stdoutTTY bool
	args      []string
}

func (r manifestRun) do(t *testing.T, files map[string]string) (stdout, stderr string) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "my-app")
	writeFiles(t, dir, files)
	t.Chdir(dir)

	launchArgs := r.args
	if r.propose {
		launchArgs = nil
	}
	ctx, _, _ := newManifestEnv().launchContext(t, dir, launchArgs...)
	if r.propose {
		flags := newPropose().Flags()
		require.NoError(t, flags.Parse(r.args))
		ctx = flagctx.NewContext(ctx, flags)
		ctx = state.WithWorkingDirectory(ctx, dir)
	}
	ctx = config.NewContext(ctx, &config.Config{Tokens: tokens.Parse("test-token")})
	t.Setenv("FLY_TRACE_COLLECTOR_URL", "127.0.0.1")

	stdoutFile, stderrFile := captureProcessOutput(t)
	ios := iostreams.FromContext(ctx)
	ios.Out, ios.ErrOut = stdoutFile, stderrFile
	if r.stdoutTTY {
		ios.SetStdoutTTY(true)
	}
	ctx = logger.NewContext(ctx, logger.New(stderrFile, logger.Info, false))

	termLogger := terminal.DefaultLogger
	if r.propose {
		require.NoError(t, runPropose(ctx))
	} else {
		require.NoError(t, run(ctx))
	}
	assert.Same(t, stdoutFile, os.Stdout, "os.Stdout not restored")
	assert.Same(t, termLogger, terminal.DefaultLogger, "terminal logger not restored")

	return string(readFile(t, stdoutFile)), string(readFile(t, stderrFile))
}

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
