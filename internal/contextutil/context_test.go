package contextutil

import (
	"context"
	"errors"
	"testing"
)

func TestCleanupPreservesParentCause(t *testing.T) {
	failure := errors.New("machine update failed")
	parent, fail := context.WithCancelCause(context.Background())
	defer fail(nil)
	ctx, cleanup := WithCancel(parent, "lease refresh stopped")
	child, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	fail(failure)
	cleanup()
	if context.Cause(ctx) != failure || context.Cause(child) != failure {
		t.Fatalf("cleanup replaced parent failure: ctx=%v child=%v", context.Cause(ctx), context.Cause(child))
	}
	if !errors.Is(child.Err(), context.Canceled) {
		t.Fatalf("child error = %v", child.Err())
	}
}

func TestCleanupPropagates(t *testing.T) {
	ctx, cleanup := WithCancel(context.Background(), "lease refresh stopped")
	child, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	cleanup()
	cleanup()
	var cause CleanupCause
	if !errors.As(context.Cause(child), &cause) || cause != "lease refresh stopped" {
		t.Fatalf("child cause = %v", context.Cause(child))
	}
	if !errors.Is(context.Cause(child), context.Canceled) {
		t.Fatalf("cause lost cancellation compatibility")
	}
}
