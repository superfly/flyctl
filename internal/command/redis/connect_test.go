//go:build !windows

package redis

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

	require.NoError(t, runRedisCLI(t.Context(), ios, script(`echo "$1 $2 $REDISCLI_AUTH"`), "16379", "secret"))
	require.Equal(t, "-p 16379 secret\n", out.String())
	require.ErrorContains(t, runRedisCLI(t.Context(), ios, script("exit 3"), "16379", "secret"), "exit status 3")
	require.Error(t, runRedisCLI(t.Context(), ios, filepath.Join(t.TempDir(), "missing"), "16379", "secret"))

	// Interrupting the session isn't a failure.
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, runRedisCLI(canceled, ios, script("exit 3"), "16379", "secret"))
}
