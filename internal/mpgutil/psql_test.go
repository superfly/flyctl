//go:build !windows

package mpgutil

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/iostreams"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output rejected") }

func TestWaitPsql(t *testing.T) {
	for _, tt := range []struct {
		name, script                 string
		terminal, noPrompt, scripted bool
		rejectOutput                 bool
		wantErr                      string
	}{
		{name: "exit 3", script: "printf 'query output\\n'\nexit 3", scripted: true, wantErr: "exit status 3"},
		{name: "output cannot be written", script: "printf 'query output\\n'", scripted: true, rejectOutput: true, wantErr: "output rejected"},
		{name: "SIGTERM on a terminal", script: "kill -TERM $$", terminal: true},
		{name: "SIGTERM on a terminal with prompts disabled", script: "kill -TERM $$", terminal: true, noPrompt: true},
		{name: "SIGTERM on a terminal, scripted", script: "kill -TERM $$", terminal: true, scripted: true, wantErr: "signal: terminated"},
		{name: "SIGTERM without a terminal", script: "kill -TERM $$", wantErr: "signal: terminated"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "psql")
			require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+tt.script+"\n"), 0700))
			streams, _, out, _ := iostreams.Test()
			streams.SetStdinTTY(tt.terminal)
			streams.SetStdoutTTY(tt.terminal)
			streams.SetNeverPrompt(tt.noPrompt)
			cmd := exec.Command(path)
			cmd.Stdout, cmd.Stderr, cmd.Stdin = streams.Out, streams.ErrOut, streams.In
			if tt.rejectOutput {
				cmd.Stdout = failingWriter{}
			}
			require.NoError(t, cmd.Start())
			t.Cleanup(func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			// The fixture uses only shell builtins, so it has no descendants.
			onTerminal := streams.IsStdinTTY() && streams.IsStdoutTTY()
			err := waitPsql(cmd, make(chan struct{}), onTerminal, tt.scripted)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
			if tt.name == "exit 3" {
				require.Equal(t, "query output\n", out.String())
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				require.Same(t, cmd.ProcessState, exitErr.ProcessState)
			}
		})
	}
}

// TestRunPsql runs the whole runner against a fake psql that records its
// arguments and then ends the way the case asks.
func TestRunPsql(t *testing.T) {
	const url = "postgresql://user:password@localhost:16380/db"

	for _, tt := range []struct {
		name               string
		ending             string
		terminal, noPrompt bool
		options            PsqlOptions
		wantArgs           string
		wantErr            string
	}{
		{name: "interactive session", ending: "exit 0", terminal: true, wantArgs: url},
		{name: "scripted", ending: "exit 0", options: PsqlOptions{Commands: []string{"select 1"}, ExtraArgs: []string{"-At"}}, wantArgs: "--dbname " + url + " -v ON_ERROR_STOP=1 -X -c select 1 -At"},
		{name: "scripted failure", ending: "exit 3", options: PsqlOptions{Files: []string{"a.sql"}}, wantArgs: "--dbname " + url + " -v ON_ERROR_STOP=1 -X -f a.sql", wantErr: "exit status 3"},
		{name: "killed on a terminal", ending: "kill -KILL $$", terminal: true, wantArgs: url},
		{name: "killed on a terminal with prompts disabled", ending: "kill -KILL $$", terminal: true, noPrompt: true, wantArgs: url},
		{name: "killed on a terminal, scripted", ending: "kill -KILL $$", terminal: true, options: PsqlOptions{Commands: []string{"select 1"}}, wantArgs: "--dbname " + url + " -v ON_ERROR_STOP=1 -X -c select 1", wantErr: "signal: killed"},
		{name: "killed without a terminal", ending: "kill -KILL $$", wantArgs: url, wantErr: "signal: killed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			argsFile := filepath.Join(dir, "args")
			path := filepath.Join(dir, "psql")
			// Only shell builtins, so the fixture has no descendants.
			script := "#!/bin/sh\necho \"$*\" > " + argsFile + "\n" + tt.ending + "\n"
			require.NoError(t, os.WriteFile(path, []byte(script), 0700))

			streams, _, _, _ := iostreams.Test()
			streams.SetStdinTTY(tt.terminal)
			streams.SetStdoutTTY(tt.terminal)
			streams.SetNeverPrompt(tt.noPrompt)
			ctx := iostreams.NewContext(context.Background(), streams)

			err := RunPsql(ctx, path, url, tt.options)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}

			args, readErr := os.ReadFile(argsFile)
			require.NoError(t, readErr)
			require.Equal(t, tt.wantArgs+"\n", string(args))
		})
	}
}
