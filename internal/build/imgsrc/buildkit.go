package imgsrc

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth"
	"github.com/moby/buildkit/util/progress/progressui"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func buildkitEnabled(docker *dockerclient.Client) (buildkitEnabled bool, err error) {
	ping, err := docker.Ping(context.Background())
	if err != nil {
		return false, err
	}

	buildkitEnabled = ping.BuilderVersion == types.BuilderBuildKit
	if buildkitEnv := os.Getenv("DOCKER_BUILDKIT"); buildkitEnv != "" {
		buildkitEnabled, err = strconv.ParseBool(buildkitEnv)
		if err != nil {
			return false, fmt.Errorf("DOCKER_BUILDKIT environment variable expects boolean value: %w", err)
		}
	}

	return buildkitEnabled, nil
}

func newBuildkitAuthProvider(tokenGetter func() string) session.Attachable {
	return &buildkitAuthProvider{
		tokenGetter: tokenGetter,
	}
}

type buildkitAuthProvider struct {
	tokenGetter func() string
}

func (ap *buildkitAuthProvider) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, ap)
}

func (ap *buildkitAuthProvider) Credentials(ctx context.Context, req *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	token := ""
	if ap.tokenGetter != nil {
		token = ap.tokenGetter()
	}
	auths := authConfigs(token)
	res := &auth.CredentialsResponse{}
	if a, ok := auths[req.Host]; ok {
		res.Username = a.Username
		res.Secret = a.Password
	}

	return res, nil
}

func (ap *buildkitAuthProvider) FetchToken(ctx context.Context, req *auth.FetchTokenRequest) (*auth.FetchTokenResponse, error) {
	return nil, status.Errorf(codes.Unavailable, "client side tokens disabled")
}

func (ap *buildkitAuthProvider) GetTokenAuthority(ctx context.Context, req *auth.GetTokenAuthorityRequest) (*auth.GetTokenAuthorityResponse, error) {
	return nil, status.Errorf(codes.Unavailable, "client side tokens disabled")
}

func (ap *buildkitAuthProvider) VerifyTokenAuthority(ctx context.Context, req *auth.VerifyTokenAuthorityRequest) (*auth.VerifyTokenAuthorityResponse, error) {
	return nil, status.Errorf(codes.Unavailable, "client side tokens disabled")
}

func newDisplay(statusCh chan *client.SolveStatus) func() error {
	return func() error {
		display, err := progressui.NewDisplay(os.Stderr, progressui.DisplayMode(os.Getenv("BUILDKIT_PROGRESS")))
		if err != nil {
			return err
		}

		// UpdateFrom must not use the incoming context.
		// Cancelling this context kills the reader of statusCh which blocks buildkit.Client's Solve() indefinitely.
		// Solve() closes statusCh at the end and UpdateFrom returns by reading the closed channel.
		//
		// See https://github.com/superfly/flyctl/pull/2682 for the context.
		_, err = display.UpdateFrom(context.Background(), statusCh)

		return err

	}
}

// Keep recovery advice at the failed upgrade: classic Docker and buildpacks
// can still use an older builder's HTTP service without BuildKit RPCs.
func dialDockerBuildkit(ctx context.Context, docker *dockerclient.Client) (net.Conn, error) {
	conn, err := docker.DialHijack(ctx, "/grpc", "h2c", nil)
	if err != nil && err.Error() == "unable to upgrade to h2c, received 500" {
		host, parseErr := url.Parse(docker.DaemonHost())
		if parseErr == nil && host.Scheme == "https" && strings.HasSuffix(host.Hostname(), ".fly.dev") {
			err = fmt.Errorf("%w; if this is an older organization builder, try WireGuard (--wg=true where supported) if it was not already tried; otherwise resolve the original WireGuard failure first. For an organization-builder deploy, add --build-only --recreate-builder to the same fly deploy command after other builds finish; this replaces the builder and discards its build cache", err)
		}
	}

	return conn, err
}
