//go:build integration

package testlib

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
)

func machineListEnv(t *testing.T, response string) *FlyctlTestEnv {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "flyctl")
	script := `#!/bin/sh
if [ "$*" != "machines list --app test-app --json" ]; then
  echo "unexpected command: $*" >&2
  exit 78
fi
count=0
if [ -f calls ]; then read count < calls; fi
count=$((count + 1))
echo "$count" > calls
` + response
	require.NoError(t, os.WriteFile(bin, []byte(script), 0700))

	return newLocalEnv(t, bin)
}

func TestMachinePollingTransientReadAndStaleConfig(t *testing.T) {
	f := machineListEnv(t, `case "$count" in
1) echo 'regions unavailable' >&2; exit 1 ;;
2) echo '[]' ;;
3) echo '[{"id":"wanted","config":{"image":"old"}}]' ;;
*) echo '[{"id":"wanted","config":{"image":"new"}}]' ;;
esac
`)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	machines, err := f.waitForMachines(ctx, "test-app", func(c assert.TestingT, machines []*fly.Machine) {
		if assert.Len(c, machines, 1) && assert.NotNil(c, machines[0].Config) {
			assert.Equal(c, "wanted", machines[0].ID)
			assert.Equal(c, "new", machines[0].Config.Image)
		}
	}, time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "new", machines[0].Config.Image)
	require.Len(t, f.history.snapshot(), 4, "retry reads until the full assertion passes")
}

func TestMachinePollingNeverAcceptsPermanentFailure(t *testing.T) {
	for _, tc := range []struct{ name, response, want string }{
		{"read error", "echo 'regions unavailable' >&2; exit 1", "regions unavailable"},
		{"wrong count", "echo '[]'", "should have 1 item"},
		{"wrong config", `echo '[{"id":"wanted","config":{"image":"old"}}]'`, "new"},
		{"malformed JSON", "echo 'broken'", "invalid machine list JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := machineListEnv(t, tc.response)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			// Once two reads are recorded, the first result has been processed.
			// Cancel then so slow subprocess startup cannot erase the test evidence.
			done := make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if len(f.history.snapshot()) >= 2 {
							cancel()
							return
						}
					}
				}
			}()
			defer func() { cancel(); <-done }()
			machines, err := f.waitForMachines(ctx, "test-app", func(c assert.TestingT, machines []*fly.Machine) {
				if assert.Len(c, machines, 1) && assert.NotNil(c, machines[0].Config) {
					assert.Equal(c, "new", machines[0].Config.Image)
				}
			}, time.Millisecond)
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, machines)
			if tc.name == "malformed JSON" {
				require.Len(t, f.history.snapshot(), 1, "malformed output fails immediately")
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestMachinePollingStopsBlockedRead(t *testing.T) {
	f := machineListEnv(t, "exec /bin/sleep 30")
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := f.waitForMachines(ctx, "test-app", func(assert.TestingT, []*fly.Machine) {
		t.Error("failed read must not reach assertions")
	}, time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 2*time.Second)
	require.Len(t, f.history.snapshot(), 1)
}

func TestMachinePollingEmptyListCanBeExpected(t *testing.T) {
	f := machineListEnv(t, "echo '[]'")
	machines, err := f.waitForMachines(t.Context(), "test-app", func(c assert.TestingT, machines []*fly.Machine) {
		assert.Empty(c, machines)
	}, time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, machines)
	require.Len(t, f.history.snapshot(), 1)
}

func TestMachinePollingCanceledBeforeStart(t *testing.T) {
	f := machineListEnv(t, "exit 0")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	res := f.FlyContextAndConfig(ctx, FlyCmdConfig{NoAssertSuccessfulExit: true, NoAssertDeadline: true}, "machines list --app test-app --json")
	require.Equal(t, -1, res.ExitCode())
	require.Contains(t, res.StdErr().String(), context.Canceled.Error())
}
