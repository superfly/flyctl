package imgsrc

import (
	"context"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
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
