package mpgutil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/iostreams"
)

func TestPsqlProxyContextPreservesOriginal(t *testing.T) {
	streams, _, out, errOut := iostreams.Test()
	streams.SetStdinTTY(true)
	streams.SetStdoutTTY(true)
	ctx := iostreams.NewContext(context.Background(), streams)
	require.Same(t, streams, iostreams.FromContext(PsqlProxyContext(ctx, PsqlOptions{})))
	redirected := iostreams.FromContext(PsqlProxyContext(ctx, PsqlOptions{Commands: []string{""}}))
	require.Same(t, errOut, redirected.Out)
	require.Same(t, out, streams.Out)
	require.True(t, streams.IsInteractive())
}

const psqlTestURL = "postgresql://user:password@localhost:16380/db"

type psqlTestCase struct {
	name    string
	options PsqlOptions
	args    []string
}

func psqlTestCases() []psqlTestCase {
	const url = psqlTestURL

	return []psqlTestCase{
		{name: "no flags or extra arguments", args: []string{url}},
		{name: "pass through -c without flags", options: PsqlOptions{ExtraArgs: []string{"-c", "bad sql"}}, args: []string{"--dbname", url, "-c", "bad sql"}},
		{name: "single command", options: PsqlOptions{Commands: []string{"select 1"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-c", "select 1"}},
		{name: "repeated commands preserve commas and order", options: PsqlOptions{Commands: []string{"select 1, 2", "select 3"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-c", "select 1, 2", "-c", "select 3"}},
		{name: "file", options: PsqlOptions{Files: []string{"schema.sql"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-f", "schema.sql"}},
		{name: "repeated files", options: PsqlOptions{Files: []string{"a.sql", "b.sql"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-f", "a.sql", "-f", "b.sql"}},
		{name: "file stdin", options: PsqlOptions{Files: []string{"-"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-f", "-"}},
		{name: "pass through overrides defaults", options: PsqlOptions{Commands: []string{"select 1"}, ExtraArgs: []string{"--csv", "-t", "-v", "ON_ERROR_STOP=0"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-c", "select 1", "--csv", "-t", "-v", "ON_ERROR_STOP=0"}},
		{name: "pass through alone keeps interactive defaults", options: PsqlOptions{ExtraArgs: []string{"--csv"}}, args: []string{"--dbname", url, "--csv"}},
		{name: "command with SQL that would fail", options: PsqlOptions{Commands: []string{"bad sql"}}, args: []string{"--dbname", url, "-v", "ON_ERROR_STOP=1", "-X", "-c", "bad sql"}},
	}
}

func TestPsqlArgv(t *testing.T) {
	for _, tt := range psqlTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.args, tt.options.args(psqlTestURL))
		})
	}
}

func TestPsqlResult(t *testing.T) {
	exitErr := errors.New("exit status 3")
	signalErr := errors.New("signal: killed")
	copyErr := errors.New("output rejected")

	for _, tt := range []struct {
		name                 string
		success, signaled    bool
		onTerminal, scripted bool
		waitErr              error
		want                 error
	}{
		{name: "success", success: true},
		{name: "success despite our cancellation", success: true, waitErr: context.Canceled},
		{name: "success but output could not be written", success: true, waitErr: copyErr, want: copyErr},
		{name: "failure on a terminal", onTerminal: true, waitErr: exitErr, want: exitErr},
		{name: "failure scripted", scripted: true, waitErr: exitErr, want: exitErr},
		{name: "signal on a terminal", signaled: true, onTerminal: true, waitErr: signalErr},
		{name: "signal on a terminal after our cancellation", signaled: true, onTerminal: true, waitErr: context.Canceled},
		{name: "signal on a terminal, scripted", signaled: true, onTerminal: true, scripted: true, waitErr: signalErr, want: signalErr},
		{name: "signal without a terminal", signaled: true, waitErr: signalErr, want: signalErr},
		{name: "signal without a terminal, scripted", signaled: true, scripted: true, waitErr: signalErr, want: signalErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, psqlResult(tt.success, tt.signaled, tt.onTerminal, tt.scripted, tt.waitErr))
		})
	}
}
