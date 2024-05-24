package imgsrc

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

func TestHeartbeat(t *testing.T) {
	dc, err := client.NewClientWithOpts()
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
			return nil
		},
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.NoError(t, err)
}

func TestStartHeartbeatFirstRetry(t *testing.T) {
	ctx := context.Background()
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: &tokens.Tokens{},
	})

	dc, err := client.NewClientWithOpts()
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
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.NoError(t, err)
}

func TestStartHeartbeatNoEndpoint(t *testing.T) {
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
				StatusCode: http.StatusNotFound,
			}
		},
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
	}

	_, err = resolver.StartHeartbeat(ctx)
	assert.Error(t, err)
}
