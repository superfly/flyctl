//go:build !windows

package redis

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	tracecmd "github.com/superfly/flyctl/internal/command/trace"
	"github.com/superfly/flyctl/iostreams"
)

// redis-cli is looked up before the user picks a database: the context has no
// API clients.
func TestConnectRequiresRedisCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ios, _, out, _ := iostreams.Test()

	err := runConnect(iostreams.NewContext(t.Context(), ios))
	require.ErrorContains(t, err, "could not find redis-cli")
	require.ErrorContains(t, err, "fly redis proxy")
	require.Empty(t, out.String())
}

func TestRunRedisCLIReportsFailure(t *testing.T) {
	script := func(body string) string {
		path := filepath.Join(t.TempDir(), "redis-cli")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))

		return path
	}
	ios, _, out, _ := iostreams.Test()

	t.Setenv("HOME", "/home/test")
	require.NoError(t, runRedisCLI(t.Context(), ios, script(`echo "$1 $2 $REDISCLI_AUTH $HOME"`), "16379", "secret"))
	require.Equal(t, "-p 16379 secret /home/test\n", out.String())

	// redis-cli's own status comes through, without another error message.
	var exit *tracecmd.ExitError
	require.ErrorAs(t, runRedisCLI(t.Context(), ios, script("exit 3"), "16379", "secret"), &exit)
	require.Equal(t, 3, exit.Code)

	require.ErrorContains(t, runRedisCLI(t.Context(), ios, filepath.Join(t.TempDir(), "missing"), "16379", "secret"), "could not run redis-cli")

	// Interrupting the session isn't a failure, whether redis-cli or flyctl
	// sees Ctrl-C first.
	require.NoError(t, runRedisCLI(t.Context(), ios, script("kill -INT $$"), "16379", "secret"))
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, runRedisCLI(canceled, ios, script("exit 3"), "16379", "secret"))
}
