package deploy_test

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
)

//go:embed testdata
var testdata embed.FS

func TestCommand_Execute(t *testing.T) {
	t.Skip("in progress")

	dir := t.TempDir()
	fsys, _ := fs.Sub(testdata, "testdata/basic")
	if err := copyFS(fsys, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil { // TODO: Revert working directory
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := deploy.New()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--image", "registry.fly.io/my-image:deployment-00000000000000000000000000"})

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &buf, ErrOut: &buf})
	ctx = task.NewWithContext(ctx)
	ctx = logger.NewContext(ctx, logger.New(&buf, logger.Info, true))

	var genqlient mock.GraphQLClient
	var client mock.Client
	client.GenqClientFunc = func() genq.Client { return &genqlient }
	ctx = flyutil.NewContextWithClient(ctx, &client)

	var flapsClient mock.FlapsClient
	ctx = flapsutil.NewContextWithClient(ctx, &flapsClient)

	client.AuthenticatedFunc = func() bool { return true }
	client.GetCurrentUserFunc = func(ctx context.Context) (*fly.User, error) {
		return &fly.User{ID: "USER1"}, nil
	}
	client.GetAppCompactFunc = func(ctx context.Context, appName string) (*fly.AppCompact, error) {
		if got, want := appName, "test-basic"; got != want {
			t.Fatalf("appName=%s, want %s", got, want)
		}
		return &fly.AppCompact{}, nil // TODO
	}

	genqlient.MakeRequestFunc = func(ctx context.Context, req *genq.Request, resp *genq.Response) error {
		vars, _ := json.Marshal(req.Variables)

		switch req.OpName {
		case "ResolverCreateBuild":
			if got, want := string(vars), `-`; got != want {
				t.Fatalf("unexpected vars: %s", vars)
			}
			// resp.Data =
		}
		return nil
	}

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
