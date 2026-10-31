//go:build integration
// +build integration

package testlib

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Collect returns a view of the env whose failures go to c instead of the
// parent test. Use it for every flyctl command run inside a
// require.EventuallyWithT condition.
//
// EventuallyWithT runs the condition on its own goroutine and, on timeout,
// returns without waiting for a still-running tick. A command that fails
// the parent test from there either aborts the whole test on a transient
// error the retry loop should have absorbed, or panics with "Fail in
// goroutine after Test has completed" once the parent has moved on. With
// the collector, a failed command just fails that tick and the loop retries.
//
// Commands still land in the shared history, and each one is logged with
// its exit status and duration as a numbered retry attempt. History is not
// printed on a failed tick, since failing ticks are expected; the final
// failure is reported by EventuallyWithT on the parent test as usual.
func (f *FlyctlTestEnv) Collect(c *assert.CollectT) *FlyctlTestEnv {
	view := *f
	view.t = &collectTB{TB: f.t, c: c}
	view.retrying = true
	view.noHistoryOnFail = true

	return &view
}

// collectTB is a testing.TB whose failure methods report to an
// assert.CollectT. Everything else, such as Helper, Cleanup and TempDir,
// still goes to the parent test. Logging is written to stdout because
// the parent panics on Log calls made after the test has finished, which
// a late tick can do.
type collectTB struct {
	testing.TB
	c *assert.CollectT
}

func (t *collectTB) Error(args ...any) {
	t.c.Errorf("%s", fmt.Sprintln(args...))
}

func (t *collectTB) Errorf(format string, args ...any) {
	t.c.Errorf(format, args...)
}

func (t *collectTB) Fail() {
	t.c.Errorf("failed")
}

func (t *collectTB) FailNow() {
	t.c.FailNow()
}

func (t *collectTB) Fatal(args ...any) {
	t.Error(args...)
	t.c.FailNow()
}

func (t *collectTB) Fatalf(format string, args ...any) {
	t.c.Errorf(format, args...)
	t.c.FailNow()
}

func (t *collectTB) Log(args ...any) {
	fmt.Print(fmt.Sprintln(args...))
}

func (t *collectTB) Logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}
