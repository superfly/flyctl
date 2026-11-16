//go:build !windows

package trace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/superfly/flyctl/iostreams"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestRunChildPassesTraceContextAndExitStatus(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	ctx, span := provider.Tracer("test").Start(iostreams.NewContext(context.Background(), iostreams.System()), "wrapper")
	defer span.End()

	dir := t.TempDir()
	capture := filepath.Join(dir, "parent")
	err := runChild(ctx, []string{"/bin/sh", "-c", `printf '%s' "$FLYCTL_TRACEPARENT" > "$1"; exit 23`, "child", capture})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 23 {
		t.Fatalf("want exit 23, got %v", err)
	}
	parent, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	sc := span.SpanContext()
	want := "00-" + sc.TraceID().String() + "-" + sc.SpanID().String() + "-01"
	if string(parent) != want {
		t.Fatalf("traceparent = %q, want %q", parent, want)
	}
}

func TestRunChildForwardsInterruptAndPreservesSuccessfulCancellation(t *testing.T) {
	streams := iostreams.System()
	streams.In = nil // Exercise the separate process group used by the deployer.
	ctx := iostreams.NewContext(context.Background(), streams)
	ready := filepath.Join(t.TempDir(), "ready")
	done := make(chan error, 1)

	go func() {
		done <- runChild(ctx, []string{"/bin/sh", "-c", `trap 'exit 0' INT; echo "$$" > "$1"; while :; do sleep 1; done`, "child", ready})
	}()

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			t.Fatalf("child exited before interrupt: %v", err)
		case <-deadline:
			t.Fatal("child was not ready")
		case <-ticker.C:
			if data, err := os.ReadFile(ready); err == nil && len(data) > 0 {
				goto interrupt
			}
		}
	}

interrupt:
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != 0 {
			t.Fatalf("want Ruby-style successful cancellation, got %v", err)
		}
	case <-deadline:
		t.Fatal("child did not receive interrupt")
	}
}
