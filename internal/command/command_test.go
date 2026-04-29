package command

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

func TestHasExternallySuppliedToken(t *testing.T) {
	t.Run("env var set", func(t *testing.T) {
		t.Setenv("FLY_ACCESS_TOKEN", "x")
		ctx := withAccessTokenFlagContext(context.Background(), "")

		if !hasExternallySuppliedToken(ctx) {
			t.Fatal("expected true when FLY_ACCESS_TOKEN is set")
		}
	})

	t.Run("flag set", func(t *testing.T) {
		t.Setenv("FLY_ACCESS_TOKEN", "")
		t.Setenv("FLY_API_TOKEN", "")
		ctx := withAccessTokenFlagContext(context.Background(), "tok")

		if !hasExternallySuppliedToken(ctx) {
			t.Fatal("expected true when --access-token flag is set")
		}
	})

	t.Run("neither set", func(t *testing.T) {
		t.Setenv("FLY_ACCESS_TOKEN", "")
		t.Setenv("FLY_API_TOKEN", "")
		ctx := withAccessTokenFlagContext(context.Background(), "")

		if hasExternallySuppliedToken(ctx) {
			t.Fatal("expected false when neither env nor flag is set")
		}
	})
}

// withAccessTokenFlagContext returns ctx with a flag set that has the
// access-token flag registered (and optionally pre-set). Mirrors how cobra
// constructs the flag context during command preparation.
func withAccessTokenFlagContext(ctx context.Context, value string) context.Context {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String(flagnames.AccessToken, "", "")
	if value != "" {
		_ = fs.Set(flagnames.AccessToken, value)
	}

	return flag.NewContext(ctx, fs)
}
