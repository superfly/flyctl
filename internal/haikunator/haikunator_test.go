package haikunator_test

import (
	"testing"

	"github.com/superfly/flyctl/internal/haikunator"
)

func TestHaikunator_Build(t *testing.T) {
	t.Run("Rand", func(t *testing.T) {
		b := haikunator.Haikunator().Delimiter("-")
		if b.Build() == "" {
			t.Fatal("expected haiku")
		}
	})

	t.Run("Deterministic", func(t *testing.T) {
		b := haikunator.Haikunator().Delimiter("-")
		b.RandN = func(max int) int { return 0 }
		if got, want := b.Build(), "autumn-waterfall-0"; got != want {
			t.Fatalf("name=%s, want %s", got, want)
		}
	})
}

func TestHaikunator_TrimSuffix(t *testing.T) {
	b := haikunator.Haikunator().Delimiter("-")

	t.Run("HaikuOnly", func(t *testing.T) {
		if got, want := b.TrimSuffix("rough-snowflake-1234"), ""; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("HaikuSuffix", func(t *testing.T) {
		if got, want := b.TrimSuffix("foobar-rough-snowflake-1234"), "foobar"; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("NoAdjective", func(t *testing.T) {
		if got, want := b.TrimSuffix("foo-snowflake-1234"), "foo-snowflake-1234"; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("NoNoun", func(t *testing.T) {
		if got, want := b.TrimSuffix("rough-foo-1234"), "rough-foo-1234"; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("NoNumber", func(t *testing.T) {
		if got, want := b.TrimSuffix("rough-snowflake-1234x"), "rough-snowflake-1234x"; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}
