//go:build !windows

package sprites

import (
	"io"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractiveSessionDetachesOnSIGQUIT(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	keys, typing := io.Pipe()
	defer typing.Close() //nolint:errcheck

	atTerminal(t, &fakeTerminal{input: keys, rows: 24, cols: 80})

	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	api.exec = func(c *execConn) {
		close(started)
		<-release
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--tty", "bash")
	e.ios.SetStdinTTY(true)
	e.ios.SetStdoutTTY(true)

	done := make(chan error, 1)
	go func() { done <- runExec(e.ctx, api.session()) }()

	<-started

	// Ctrl+\ in a terminal that is not yet raw: the process gets SIGQUIT.
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGQUIT))

	select {
	case err := <-done:
		require.NoError(t, err, "detaching is not a failure")
	case <-time.After(10 * time.Second):
		t.Fatal("SIGQUIT did not detach")
	}

	assert.Contains(t, e.errOut.String(), "Detached.")
}
