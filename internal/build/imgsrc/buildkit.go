package imgsrc

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth"
	"github.com/superfly/fly-go"
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

func newBuildkitAuthProvider(ctx context.Context, app *fly.AppCompact) session.Attachable {
	return &buildkitAuthProvider{
		ctx: ctx,
		app: app,
	}
}

type buildkitAuthProvider struct {
	ctx context.Context
	app *fly.AppCompact
}

func (ap *buildkitAuthProvider) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, ap)
}

func (ap *buildkitAuthProvider) Credentials(ctx context.Context, req *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	var token string
	var err error

	if req.Host == registryHostForAuth {
		// We need the build context to get a build token
		buildCtx, cancel := context.WithCancelCause(ap.ctx)
		// Cancel the build token request when the gRPC context is cancelled
		stop := context.AfterFunc(ctx, func() {
			cancel(context.Cause(ctx))
		})
		defer stop()

		token, err = getBuildToken(buildCtx, ap.app)
		if err != nil {
			return nil, status.Errorf(codes.Unknown, "failed to get build token: %s", err)
		}
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
