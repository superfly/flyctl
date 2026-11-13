package flyerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestExpected(t *testing.T) {
	sentinel := Expected(errors.New("deployment aborted by user"))
	wrapped := fmt.Errorf("deploy: %w", sentinel)

	if !IsExpected(wrapped) {
		t.Fatal("IsExpected lost the mark through wrapping")
	}
	if !errors.Is(wrapped, sentinel) {
		t.Fatal("errors.Is no longer finds the marked sentinel")
	}
	if got := wrapped.Error(); got != "deploy: deployment aborted by user" {
		t.Fatalf("message changed: %q", got)
	}
	if IsExpected(errors.New("deployment aborted by user")) {
		t.Fatal("unmarked error reported as expected")
	}
	if Expected(nil) != nil {
		t.Fatal("Expected(nil) must stay nil")
	}
}
