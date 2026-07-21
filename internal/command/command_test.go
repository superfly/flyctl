package command

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

func TestFilesFromCommandUsesPOSIXGuestPath(t *testing.T) {
	content := []byte("hello from windows")
	localPath := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(localPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.StringArray("file-local", []string{"/etc/config.txt=" + localPath}, "")
	ctx := flag.NewContext(context.Background(), fs)

	files, err := FilesFromCommand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if got, want := files[0].GuestPath, "/etc/config.txt"; got != want {
		t.Errorf("GuestPath = %q, want %q", got, want)
	}
	if files[0].RawValue == nil {
		t.Fatal("RawValue is nil")
	}
	if got, want := *files[0].RawValue, base64.StdEncoding.EncodeToString(content); got != want {
		t.Errorf("RawValue = %q, want %q", got, want)
	}
}

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
