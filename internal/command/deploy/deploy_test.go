package deploy

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/inmem"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

//go:embed testdata
var testdata embed.FS

func TestCommand_Execute(t *testing.T) {
	makeTerminalLoggerQuiet(t)
	const (
		imageTag       = "test-registry.fly.io/my-image:deployment-00000000000000000000000000"
		imageDigest    = "sha256:f107dbfaa732063b31ee94aa728c4f5648a672259fd62bfaa245f9b7a53b5479"
		imageReference = imageTag + "@" + imageDigest
	)

	// Set FLY_ACCESS_TOKEN to simulate CI/CD environment
	t.Setenv("FLY_ACCESS_TOKEN", "test-token")

	dir := t.TempDir()
	fsys, _ := fs.Sub(testdata, "testdata/basic")
	if err := copyFS(fsys, dir); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	var buf bytes.Buffer
	cmd := New()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--image", imageTag})

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &buf, ErrOut: &buf})
	ctx = task.NewWithContext(ctx)
	ctx = logger.NewContext(ctx, logger.New(&buf, logger.Info, true))

	// Set up config with LastLogin timestamp to satisfy session timeout check
	cfg := &config.Config{
		Tokens:    tokens.Parse("test-token"),
		LastLogin: time.Now(),
	}
	ctx = config.NewContext(ctx, cfg)

	server := inmem.NewServer()
	server.CreateApp(&fly.App{
		Name:         "test-basic",
		Organization: fly.Organization{Slug: "my-org"},
	})
	if err := server.CreateImage(context.Background(), "test-basic", imageTag, &fly.Image{
		ID:             "IMAGE1",
		Ref:            imageTag,
		Digest:         imageDigest,
		CompressedSize: "1000",
	}); err != nil {
		t.Fatal(err)
	}

	ctx = flyutil.NewContextWithClient(ctx, server.Client())
	ctx = flapsutil.NewContextWithClient(ctx, server.FlapsClient("test-basic"))
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}

	machines, err := server.FlapsClient("test-basic").ListActive(ctx, "test-basic")
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) == 0 {
		t.Fatal("expected at least one active app machine")
	}
	for _, machine := range machines {
		if got := machine.Config.Image; got != imageReference {
			t.Fatalf("expected immutable deployment image %q, got %q", imageReference, got)
		}
	}
}

// copyFS writes the contents of a file system to a destination path on disk.
func copyFS(fsys fs.FS, dst string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		target := filepath.Join(dst, filepath.FromSlash(path))
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, b, 0o666)
	})
}

func chdir(tb testing.TB, dir string) {
	tb.Helper()

	prev, err := os.Getwd()
	if err != nil {
		tb.Fatalf("cannot read working directory: %s", err)
	}
	if err := os.Chdir(dir); err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		tb.Helper()
		if err := os.Chdir(prev); err != nil {
			tb.Fatalf("cannot revert working directory: %s", err)
		}
	})
}
