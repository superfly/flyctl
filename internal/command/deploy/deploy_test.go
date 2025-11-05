package deploy

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/inmem"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
)

//go:embed testdata
var testdata embed.FS

func TestCommand_Execute(t *testing.T) {
	makeTerminalLoggerQuiet(t)

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
	cmd.SetArgs([]string{"--image", "test-registry.fly.io/my-image:deployment-00000000000000000000000000"})

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &buf, ErrOut: &buf})
	ctx = task.NewWithContext(ctx)
	ctx = logger.NewContext(ctx, logger.New(&buf, logger.Info, true))

	server := inmem.NewServer()
	server.CreateApp(&fly.App{
		Name:         "test-basic",
		Organization: fly.Organization{Slug: "my-org"},
	})
	if err := server.CreateImage(context.Background(), "test-basic", "test-registry.fly.io/my-image:deployment-00000000000000000000000000", &fly.Image{
		ID:             "IMAGE1",
		Ref:            "test-registry.fly.io/my-image:deployment-00000000000000000000000000",
		CompressedSize: "1000",
	}); err != nil {
		t.Fatal(err)
	}

	ctx = flyutil.NewContextWithClient(ctx, server.Client())
	ctx = flapsutil.NewContextWithClient(ctx, server.FlapsClient("test-basic"))

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
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
