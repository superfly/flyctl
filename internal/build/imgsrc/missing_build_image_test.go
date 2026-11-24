package imgsrc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	dockerclient "github.com/docker/docker/client"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

type resolveImageFunc func(context.Context, string, string) (*fly.Image, error)

func (f resolveImageFunc) ResolveImageForApp(ctx context.Context, app, ref string) (*fly.Image, error) {
	return f(ctx, app, ref)
}

func TestResolveMissingBuildImage(t *testing.T) {
	manifest := digest.FromString("manifest")
	other := digest.FromString("other")
	opts := ImageOptions{AppName: "app", Tag: "registry.fly.io/app:build"}
	for _, tc := range []struct {
		name            string
		receipt         digest.Digest
		buildID, remote digest.Digest
		buildkit        bool
		lookup          string
		recover         bool
	}{
		{"push receipt identifies exact manifest", manifest, other, manifest, true, opts.Tag + "@" + manifest.String(), true},
		{"receipt and registry disagree", manifest, other, other, true, opts.Tag + "@" + manifest.String(), false},
		{"BuildKit manifest matches pushed tag", "", manifest, manifest, true, opts.Tag, true},
		{"tag moved after push", "", manifest, other, true, opts.Tag, false},
		{"classic build without receipt", "", other, other, false, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			api := resolveImageFunc(func(_ context.Context, app, ref string) (*fly.Image, error) {
				called = true
				require.Equal(t, opts.AppName, app)
				require.Equal(t, tc.lookup, ref)

				return &fly.Image{Digest: tc.remote.String(), CompressedSize: "123"}, nil
			})
			img, err := resolveMissingBuildImage(context.Background(), api, opts, io.Discard, tc.buildID.String(), tc.receipt, tc.buildkit)
			require.NoError(t, err)
			require.Equal(t, tc.lookup != "", called)
			if !tc.recover {
				require.Nil(t, img)

				return
			}
			require.Equal(t, opts.Tag+"@"+manifest.String(), img.Tag)
			require.Equal(t, img.Tag, img.String())
			require.EqualValues(t, 123, img.Size)
		})
	}
}

func TestFindTaggedBuildImageAfterLocalLoss(t *testing.T) {
	opts := ImageOptions{Tag: "registry.fly.io/app:build", AppName: "app"}
	ack := digest.FromString("pushed manifest")
	for _, tc := range []struct {
		name             string
		receipt, current digest.Digest
	}{
		{"matching receipt", ack, ack},
		{"moved tag", ack, digest.FromString("replacement")},
		{"inspection denied", ack, ""},
		{"no receipt", "", ack},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := config.NewContext(context.Background(), &config.Config{Tokens: tokens.Parse("")})
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1.41/images/json":
					_, _ = io.WriteString(w, "[]")
				case "/v1.41/distribution/" + opts.Tag + "/json":
					if tc.receipt == "" {
						t.Error("queried registry without a push receipt")
					}
					if tc.current == "" {
						http.Error(w, "registry access denied", http.StatusForbidden)

						return
					}
					_, _ = fmt.Fprintf(w, `{"Descriptor":{"digest":%q}}`, tc.current)
				default:
					http.NotFound(w, r)
				}
			}))
			defer daemon.Close()
			docker, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("tcp://"+daemon.Listener.Addr().String()), dockerclient.WithVersion("1.41"))
			require.NoError(t, err)
			defer docker.Close()

			called := false
			api := resolveImageFunc(func(_ context.Context, app, ref string) (*fly.Image, error) {
				called = true
				require.Equal(t, opts.AppName, app)
				require.Equal(t, opts.Tag+"@"+ack.String(), ref)
				return &fly.Image{Digest: ack.String(), ID: "registry-id", CompressedSize: "123"}, nil
			})
			img, err := findTaggedBuildImage(ctx, docker, api, opts, io.Discard, tc.receipt)
			require.Equal(t, tc.receipt == tc.current, called)
			if tc.current == "" {
				require.ErrorContains(t, err, "registry access denied")
				require.Nil(t, img)

				return
			}
			if tc.receipt != tc.current {
				require.EqualError(t, err, "no image found")
				require.Nil(t, img)
				return
			}
			require.NoError(t, err)
			require.Equal(t, opts.Tag+"@"+ack.String(), img.Tag)
			require.Equal(t, "registry-id", img.ID)
		})
	}
}
