package imgsrc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialDockerBuildkit(t *testing.T) {
	for _, tc := range []struct {
		name, host string
		status     int
		advice     bool
	}{
		{"legacy public", "https://builder.fly.dev", 500, true},
		{"TCP endpoint", "tcp://builder.fly.dev:2375", 500, false},
		{"custom", "https://builder.example.com", 500, false},
		{"authentication", "https://builder.fly.dev", 401, false},
		{"unavailable", "https://builder.fly.dev", 503, false},
		{"success", "https://builder.fly.dev", 101, false},
		{"canceled", "https://builder.fly.dev", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.status == 0 {
				cancel()
			}
			docker, err := dockerclient.NewClientWithOpts(dockerclient.WithHost(tc.host), dockerclient.WithDialContext(func(ctx context.Context, _, _ string) (net.Conn, error) {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				client, server := net.Pipe()
				go func() {
					defer server.Close()
					req, err := http.ReadRequest(bufio.NewReader(server))
					if !assert.NoError(t, err) {
						return
					}
					assert.Equal(t, "/grpc", req.URL.Path)
					assert.Equal(t, "h2c", req.Header.Get("Upgrade"))
					fmt.Fprintf(server, "HTTP/1.1 %d %s\r\nConnection: Upgrade\r\nUpgrade: h2c\r\nContent-Length: 5\r\n\r\nproof", tc.status, http.StatusText(tc.status))
				}()

				return client, nil
			}))
			require.NoError(t, err)
			defer docker.Close()
			conn, err := dialDockerBuildkit(ctx, docker)
			switch tc.status {
			case 0:
				require.Nil(t, conn)
				require.ErrorIs(t, err, context.Canceled)
				require.NotContains(t, err.Error(), "--recreate-builder")
			case 101:
				require.NoError(t, err)
				defer conn.Close()
				data, err := io.ReadAll(conn)
				require.NoError(t, err)
				require.Equal(t, "proof", string(data))
			default:
				require.Nil(t, conn)
				original := fmt.Sprintf("unable to upgrade to h2c, received %d", tc.status)
				if tc.advice {
					require.EqualError(t, errors.Unwrap(err), original)
					require.ErrorContains(t, err, "if this is an older organization builder, try WireGuard (--wg=true where supported) if it was not already tried; otherwise resolve the original WireGuard failure first. For an organization-builder deploy, add --build-only --recreate-builder to the same fly deploy command after other builds finish; this replaces the builder and discards its build cache")
				} else {
					require.EqualError(t, err, original)
				}
			}
		})
	}
}
