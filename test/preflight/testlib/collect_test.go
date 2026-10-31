//go:build integration
// +build integration

package testlib

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingT captures what EventuallyWithT reports without failing the
// real test.
type recordingT struct {
	errors []string
}

func (r *recordingT) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recordingT) FailNow() {}

// newLocalEnv builds an env around an arbitrary binary without touching the
// platform, so the harness itself can be exercised offline.
func newLocalEnv(t *testing.T, bin string) *FlyctlTestEnv {
	return &FlyctlTestEnv{
		t:         t,
		flyctlBin: bin,
		workDir:   t.TempDir(),
		env:       map[string]string{},
		history:   &cmdHistory{},
	}
}

func TestCollectFailedCommandOnlyFailsTheTick(t *testing.T) {
	f := newLocalEnv(t, "false")

	rec := &recordingT{}
	var ticks atomic.Int32
	ok := assert.EventuallyWithT(rec, func(c *assert.CollectT) {
		ticks.Add(1)
		f.Collect(c).Fly("anything")
	}, 250*time.Millisecond, 50*time.Millisecond)

	require.False(t, ok, "a command that always fails must not satisfy the condition")
	require.Greater(t, ticks.Load(), int32(1), "the failing command must be retried, not abort the loop")
	require.NotEmpty(t, rec.errors)
	require.Contains(t, rec.errors[0], "expected successful zero exit code")
	require.False(t, t.Failed(), "the parent test must not be failed by a failing tick")
}

func TestCollectSuccessfulCommandSatisfiesCondition(t *testing.T) {
	f := newLocalEnv(t, "true")

	rec := &recordingT{}
	ok := assert.EventuallyWithT(rec, func(c *assert.CollectT) {
		res := f.Collect(c).Fly("anything")
		assert.Zero(c, res.ExitCode())
	}, time.Second, 50*time.Millisecond)

	require.True(t, ok)
	require.Empty(t, rec.errors)
}

func TestCollectSharesCommandHistory(t *testing.T) {
	f := newLocalEnv(t, "true")

	f.Fly("first")
	c := new(assert.CollectT)
	f.Collect(c).Fly("second")
	f.Collect(c).Fly("third")

	require.Len(t, f.history.snapshot(), 3, "commands run through the view must appear in the parent's history")
	require.Equal(t, 2, f.history.retries, "only commands run through the view count as retry attempts")
}

func TestCollectKeepsNonFailureMethodsOnParent(t *testing.T) {
	f := newLocalEnv(t, "true")
	view := f.Collect(new(assert.CollectT))

	require.Equal(t, t.Name(), view.t.Name())
	require.NotEmpty(t, view.t.TempDir())
}
