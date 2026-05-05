package wireguard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

// withCtx builds a minimal context with iostreams + a flag set populated with
// the given positional args. Mirrors what the cobra preparer chain produces
// for resolveOutputWriter at runtime.
func withCtx(t *testing.T, args []string) context.Context {
	t.Helper()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("flag parse: %v", err)
	}

	ctx := flag.NewContext(context.Background(), fs)
	ios, _, _, _ := iostreams.Test()

	return iostreams.NewContext(ctx, ios)
}

// runWithDeadline runs fn in a goroutine and fails the test if it doesn't
// return within d. Critical for catching infinite-loop regressions in
// resolveOutputWriter — the function under test is the same one that hung
// indefinitely in flyctl issue #4665.
func runWithDeadline(t *testing.T, d time.Duration, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("function did not return within %v — likely infinite loop regression", d)
	}
}

func TestResolveOutputWriter_CLIArg_FileExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.conf")
	if err := os.WriteFile(existing, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := withCtx(t, []string{existing})

	runWithDeadline(t, time.Second, func() {
		w, mustClose, err := resolveOutputWriter(ctx, 0, "test prompt: ")
		if err == nil {
			t.Fatal("expected error for file-exists CLI arg, got nil")
		}
		if w != nil {
			t.Errorf("expected nil writer, got %v", w)
		}
		if mustClose {
			t.Errorf("expected mustClose=false, got true")
		}
	})
}

func TestResolveOutputWriter_CLIArg_EmptyString(t *testing.T) {
	ctx := withCtx(t, []string{""})

	runWithDeadline(t, time.Second, func() {
		w, _, err := resolveOutputWriter(ctx, 0, "test prompt: ")
		if err == nil {
			t.Fatal("expected error for empty CLI arg, got nil")
		}
		if w != nil {
			t.Errorf("expected nil writer, got %v", w)
		}
	})
}

func TestResolveOutputWriter_CLIArg_Stdout(t *testing.T) {
	ctx := withCtx(t, []string{"stdout"})

	runWithDeadline(t, time.Second, func() {
		w, mustClose, err := resolveOutputWriter(ctx, 0, "test prompt: ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if w != os.Stdout {
			t.Errorf("expected os.Stdout, got %v", w)
		}
		if mustClose {
			t.Errorf("expected mustClose=false for stdout, got true")
		}
	})
}

func TestResolveOutputWriter_CLIArg_HappyPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new.conf")
	ctx := withCtx(t, []string{target})

	runWithDeadline(t, time.Second, func() {
		w, mustClose, err := resolveOutputWriter(ctx, 0, "test prompt: ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if w == nil {
			t.Fatal("expected non-nil writer")
		}
		if !mustClose {
			t.Errorf("expected mustClose=true for opened file")
		}
		if _, statErr := os.Stat(target); statErr != nil {
			t.Errorf("file not created at %s: %v", target, statErr)
		}
		if err := w.Close(); err != nil {
			t.Errorf("close error: %v", err)
		}
	})
}
