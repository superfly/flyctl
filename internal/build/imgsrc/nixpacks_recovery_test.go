package imgsrc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync/atomic"
	"testing"

	dockerclient "github.com/docker/docker/client"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestNixpacksRecoversItsPushReceipt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in nixpacks executable is a shell script")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	// With no CLI config initialized, nixpacks looks for bin/nixpacks in dir.
	require.Empty(t, flyctl.ConfigDir())
	require.NoError(t, os.Mkdir("bin", 0o700))
	require.NoError(t, os.WriteFile("bin/nixpacks", []byte("#!/bin/sh\necho \"$3\" >> builds\n"), 0o700))

	opts := ImageOptions{AppName: "app", Tag: "registry.fly.io/app:build", WorkingDir: dir, Publish: true}
	ack := digest.FromString("nixpacks manifest")
	var pushes atomic.Int32
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.41/images/registry.fly.io/app/push":
			pushes.Add(1)
			_, _ = fmt.Fprintf(w, `{"aux":{"Tag":"build","Digest":%q}}`, ack)
		case "/v1.41/images/json":
			_, _ = io.WriteString(w, "[]")
		case "/v1.41/distribution/" + opts.Tag + "/json":
			_, _ = fmt.Fprintf(w, `{"Descriptor":{"digest":%q}}`, ack)
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()

	factory := &dockerClientFactory{
		mode: DockerDaemonTypeLocal,
		buildFn: func(context.Context, *build) (*dockerclient.Client, error) {
			return dockerclient.NewClientWithOpts(dockerclient.WithHost("tcp://"+daemon.Listener.Addr().String()), dockerclient.WithVersion("1.41"))
		},
		apiClient: &mock.Client{ResolveImageForAppFunc: func(_ context.Context, app, ref string) (*fly.Image, error) {
			require.Equal(t, opts.AppName, app)
			require.Equal(t, opts.Tag+"@"+ack.String(), ref)

			return &fly.Image{Digest: ack.String(), CompressedSize: "123"}, nil
		}},
	}
	ios, _, _, _ := iostreams.Test()
	ctx := config.NewContext(iostreams.NewContext(context.Background(), ios), &config.Config{Tokens: tokens.Parse("")})
	img, _, err := (&nixpacksBuilder{}).Run(ctx, factory, ios, opts, newBuild(0, false))
	require.NoError(t, err)
	require.Equal(t, opts.Tag+"@"+ack.String(), img.Tag)
	require.EqualValues(t, 1, pushes.Load())
	builds, err := os.ReadFile("builds")
	require.NoError(t, err)
	require.Equal(t, opts.Tag+"\n", string(builds))
}
