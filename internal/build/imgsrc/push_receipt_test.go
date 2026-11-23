package imgsrc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dockerclient "github.com/docker/docker/client"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/iostreams"
)

func TestPushToFlyWithDigestReadsPushReceipt(t *testing.T) {
	manifest := digest.FromString("manifest")
	for _, tc := range []struct {
		name string
		aux  string
		want digest.Digest
	}{
		{"receipt for the pushed tag", fmt.Sprintf(`{"Tag":"build","Digest":%q,"Size":523}`, manifest), manifest},
		{"receipt for another tag", fmt.Sprintf(`{"Tag":"other","Digest":%q,"Size":523}`, manifest), ""},
		{"unrelated aux message", fmt.Sprintf(`{"manifestPushedInsteadOfIndex":true,"originalIndex":{"digest":%q}}`, manifest), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.True(t, strings.HasSuffix(r.URL.Path, "/images/registry.fly.io/app/push"), r.URL.Path)
				fmt.Fprintf(w, "{\"status\":\"Pushed\"}\n{\"aux\":%s}\n", tc.aux)
			}))
			defer daemon.Close()

			docker, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("tcp://"+daemon.Listener.Addr().String()), dockerclient.WithVersion("1.41"))
			require.NoError(t, err)
			defer docker.Close()

			ios, _, _, _ := iostreams.Test()
			ctx := config.NewContext(context.Background(), &config.Config{Tokens: tokens.Parse("")})

			got, err := pushToFlyWithDigest(ctx, docker, ios, "registry.fly.io/app:build")
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
