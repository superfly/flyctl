package ctrlc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestHookContextSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal(os.Interrupt) is unsupported on Windows")
	}
	// Isolate signal delivery from other tests and the test runner.
	if os.Getenv("FLYCTL_TEST_INTERRUPT_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHookContextSignal$")
		cmd.Env = append(os.Environ(), "FLYCTL_TEST_INTERRUPT_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("signal test: %v\n%s", err, out)
		}

		return
	}
	parent, stop := context.WithCancelCause(context.Background())
	defer stop(nil)
	ctx, cleanup := HookCancelableContext(parent, func() { stop(nil) })
	defer cleanup()
	child, cancelChild := context.WithCancelCause(ctx)
	defer cancelChild(nil)
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-child.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt did not cancel child")
	}
	cleanup()
	if ctx.Err() != AbortedByUser || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("hook error = %v", ctx.Err())
	}
	if context.Cause(ctx) != AbortedByUser || context.Cause(child) != AbortedByUser {
		t.Fatalf("causes: hook=%v child=%v", context.Cause(ctx), context.Cause(child))
	}
}

func TestHookContextPreservesEarlierCause(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint("timeout=", timeout), func(t *testing.T) {
			var parent context.Context
			var cancel context.CancelFunc
			cause := errors.New("sibling machine update failed")
			wantErr := context.Canceled
			if timeout {
				cause = fmt.Errorf("machine health checks: %w", context.DeadlineExceeded)
				parent, cancel = context.WithTimeoutCause(context.Background(), -time.Second, cause)
				wantErr = context.DeadlineExceeded
			} else {
				p, fail := context.WithCancelCause(context.Background())
				fail(cause)
				parent, cancel = p, func() { fail(nil) }
			}
			ctx, cleanup := HookCancelableContext(parent, cancel)
			cleanup()
			if ctx.Err() != wantErr || context.Cause(ctx) != cause {
				t.Fatalf("error=%v cause=%v", ctx.Err(), context.Cause(ctx))
			}
		})
	}
}
