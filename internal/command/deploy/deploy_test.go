package deploy_test

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
	dir := t.TempDir()
	fsys, _ := fs.Sub(testdata, "testdata/basic")
	if err := copyFS(fsys, dir); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	var buf bytes.Buffer
	cmd := deploy.New()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--image", "registry.fly.io/my-image:deployment-00000000000000000000000000"})

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &buf, ErrOut: &buf})
	ctx = task.NewWithContext(ctx)
	ctx = logger.NewContext(ctx, logger.New(&buf, logger.Info, true))

	var client mock.Client
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
		return &fly.AppCompact{
			Organization: &fly.OrganizationBasic{Slug: "my-org"},
		}, nil
	}
	client.CreateBuildFunc = func(ctx context.Context, input fly.CreateBuildInput) (*fly.CreateBuildResponse, error) {
		return &fly.CreateBuildResponse{
			CreateBuild: fly.CreateBuildCreateBuildCreateBuildPayload{
				Id: "BUILD1",
			},
		}, nil
	}
	client.ResolveImageForAppFunc = func(ctx context.Context, appName, imageRef string) (*fly.Image, error) {
		return &fly.Image{
			ID:             "IMAGE1",
			Ref:            "test-registry.fly.io/test-basic/deployment-123",
			CompressedSize: "1000",
		}, nil
	}
	client.FinishBuildFunc = func(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error) {
		return &fly.FinishBuildResponse{
			FinishBuild: fly.FinishBuildFinishBuildFinishBuildPayload{
				Id:              "BUILD1",
				Status:          "",
				WallclockTimeMs: 2000,
			},
		}, nil
	}
	client.CreateReleaseFunc = func(ctx context.Context, input fly.CreateReleaseInput) (*fly.CreateReleaseResponse, error) {
		return &fly.CreateReleaseResponse{}, nil
	}
	client.UpdateReleaseFunc = func(ctx context.Context, input fly.UpdateReleaseInput) (*fly.UpdateReleaseResponse, error) {
		return &fly.UpdateReleaseResponse{}, nil
	}
	client.GetIPAddressesFunc = func(ctx context.Context, appName string) ([]fly.IPAddress, error) {
		return nil, nil
	}

	flapsClient.ListFlyAppsMachinesFunc = func(ctx context.Context) ([]*fly.Machine, *fly.Machine, error) {
		return nil, nil, nil // no machines
	}
	flapsClient.ListActiveFunc = func(ctx context.Context) ([]*fly.Machine, error) {
		return nil, nil // no active machines
	}
	flapsClient.LaunchFunc = func(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
		return &fly.Machine{
			ID:     "m0",
			Region: "ord",
			Config: &fly.MachineConfig{},
		}, nil
	}
	flapsClient.GetFunc = func(ctx context.Context, machineID string) (*fly.Machine, error) {
		return &fly.Machine{
			ID:     "m0",
			Region: "ord",
			Config: &fly.MachineConfig{},
		}, nil
	}
	flapsClient.WaitFunc = func(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
		return nil
	}
	flapsClient.GetProcessesFunc = func(ctx context.Context, machineID string) (fly.MachinePsResponse, error) {
		return nil, nil
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
