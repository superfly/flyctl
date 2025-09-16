package imgsrc

import (
	"context"
	"fmt"
	"os"
	"strconv"

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
