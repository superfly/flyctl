package imgsrc

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// Deploy fails a build over to HTTPS only when it went over WireGuard, so an
// org builder reached over WireGuard must count as such even when provisioning
// fails before any connection is made.
func TestUsedWireguardAfterFailedOrgBuilder(t *testing.T) {
	cases := []struct {
		name      string
		wireguard bool
		want      bool
	}{
		{name: "over wireguard", wireguard: true, want: true},
		{name: "over https", wireguard: false, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := config.NewContext(context.Background(), &config.Config{Tokens: &tokens.Tokens{}})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
					return &flaps.App{Name: name, Organization: flaps.AppOrganizationInfo{Slug: "my-org"}}, nil
				},
			})
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
					return nil, errors.New("api unavailable")
				},
			})
			ios, _, _, _ := iostreams.Test()
			resolver := NewResolver(DockerDaemonTypeRemote, nil, "my-app", ios, tc.wireguard, false, WithProvisioner(&Provisioner{}))

			_, err := resolver.StartHeartbeat(ctx)

			require.ErrorContains(t, err, "api unavailable")
			assert.Equal(t, tc.want, resolver.UsedWireguard())
		})
	}
}

func TestDeploymentImage(t *testing.T) {
	image := &DeploymentImage{
		ID:     "img_8rlxp2nzn32np3jq",
		Tag:    "docker-hub-mirror.fly.io/flyio/postgres-flex:16",
		Digest: "sha256:f107dbfaa732063b31ee94aa728c4f5648a672259fd62bfaa245f9b7a53b5479",
		Size:   123,
	}
	assert.Equal(t, "docker-hub-mirror.fly.io/flyio/postgres-flex:16@sha256:f107dbfaa732063b31ee94aa728c4f5648a672259fd62bfaa245f9b7a53b5479", image.String())

	image.Digest = ""
	assert.Equal(t, "docker-hub-mirror.fly.io/flyio/postgres-flex:16", image.String())
}

func TestDeploymentImagePinned(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	image := &DeploymentImage{
		Tag:    "haproxy@" + digest,
		Digest: digest,
	}
	assert.Equal(t, image.Tag, image.String())

	image.Tag = "haproxy:latest@" + digest
	assert.Equal(t, image.Tag, image.String())

	image.Digest = "sha256:" + strings.Repeat("b", 64)
	assert.Equal(t, "haproxy:latest@"+image.Digest, image.String())

	image.Digest = ""
	assert.Equal(t, image.Tag, image.String())
}

func TestHeartbeat(t *testing.T) {
	dc, err := client.NewClientWithOpts(client.WithHost("tcp://127.0.0.1:2375"))
	assert.NoError(t, err)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", http.NoBody)
	assert.NoError(t, err)

	err = heartbeat(ctx, dc, req)
	assert.Error(t, err)
}

func TestStartHeartbeat(t *testing.T) {
	ctx := context.Background()
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: &tokens.Tokens{},
	})

	dc, err := client.NewClientWithOpts(client.WithHost("tcp://127.0.0.1:2375"))
	assert.NoError(t, err)

	resolver := Resolver{
		dockerFactory: &dockerClientFactory{
			remote: true,
			buildFn: func(ctx context.Context, build *build) (*client.Client, error) {
				return dc, nil
			},
			apiClient: nil,
			appName:   "myapp",
		},
		apiClient: nil,
		heartbeatFn: func(ctx context.Context, client *client.Client, req *http.Request) error {
			return nil
		},
		provisioner: &Provisioner{},
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.NoError(t, err)
}

func TestStartHeartbeatFirstRetry(t *testing.T) {
	ctx := context.Background()
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: &tokens.Tokens{},
	})

	dc, err := client.NewClientWithOpts(client.WithHost("tcp://127.0.0.1:2375"))
	assert.NoError(t, err)

	numCalls := 0

	resolver := Resolver{
		dockerFactory: &dockerClientFactory{
			remote: true,
			buildFn: func(ctx context.Context, build *build) (*client.Client, error) {
				return dc, nil
			},
			apiClient: nil,
			appName:   "myapp",
		},
		apiClient: nil,
		heartbeatFn: func(ctx context.Context, client *client.Client, req *http.Request) error {
			if numCalls == 0 {
				numCalls += 1

				return errors.New("first error")
			}

			return nil
		},
		provisioner: &Provisioner{},
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.NoError(t, err)
}

func TestStartHeartbeatNoEndpoint(t *testing.T) {
	ctx := context.Background()
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: &tokens.Tokens{},
	})

	dc, err := client.NewClientWithOpts(client.WithHost("tcp://127.0.0.1:2375"))
	assert.NoError(t, err)

	resolver := Resolver{
		dockerFactory: &dockerClientFactory{
			remote: true,
			buildFn: func(ctx context.Context, build *build) (*client.Client, error) {
				return dc, nil
			},
			apiClient: nil,
			appName:   "myapp",
		},
		apiClient: nil,
		heartbeatFn: func(ctx context.Context, client *client.Client, req *http.Request) error {
			return &httpError{
				StatusCode: http.StatusNotFound,
			}
		},
		provisioner: &Provisioner{},
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.NoError(t, err)
}

func TestStartHeartbeatWError(t *testing.T) {
	ctx := context.Background()
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: &tokens.Tokens{},
	})

	dc, err := client.NewClientWithOpts()
	assert.NoError(t, err)

	resolver := Resolver{
		dockerFactory: &dockerClientFactory{
			remote: true,
			buildFn: func(ctx context.Context, build *build) (*client.Client, error) {
				return dc, nil
			},
			apiClient: nil,
			appName:   "myapp",
		},
		apiClient: nil,
		heartbeatFn: func(ctx context.Context, client *client.Client, req *http.Request) error {
			return &httpError{
				StatusCode: http.StatusBadRequest,
			}
		},
		provisioner: &Provisioner{},
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.Error(t, err)
}
