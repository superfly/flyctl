package imgsrc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/iostreams"
)

var _ imageResolver = (*FlapsResolver)(nil)

func NewFlapsResolver(addr string) *FlapsResolver {
	return &FlapsResolver{addr: addr}
}

type FlapsResolver struct{ addr string }

func (r *FlapsResolver) Name() string { return "Flaps" }

func (r *FlapsResolver) Run(
	ctx context.Context,
	_ *dockerClientFactory,
	streams *iostreams.IOStreams,
	opts RefOptions,
	_ *build,
) (*DeploymentImage, string, error) {
	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' in flaps registry...\n", opts.ImageRef)

	resolver := docker.NewResolver(docker.ResolverOptions{Hosts: registryHosts(config.Tokens(ctx).Docker())})
	_, desc, err := resolver.Resolve(ctx, opts.ImageRef)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve image: %w", err)
	}

	return &DeploymentImage{
		ID:     desc.Digest.String(),
		Digest: desc.Digest.String(),
		Size:   desc.Size,
		Tag:    opts.ImageRef,
	}, fmt.Sprintf("Found image %s", desc.Digest.String()), nil
}

func registryHosts(token string) docker.RegistryHosts {
	return func(host string) ([]docker.RegistryHost, error) {
		if host == "registry.fly.io" {
			return []docker.RegistryHost{
				{
					Client:       &http.Client{},
					Host:         "_api.internal:4280",
					Scheme:       "http",
					Path:         "/v1/registry/v2",
					Capabilities: docker.HostCapabilityPull | docker.HostCapabilityResolve,
					Authorizer: docker.NewDockerAuthorizer(
						docker.WithAuthClient(&http.Client{}),
						docker.WithAuthCreds(func(host string) (string, string, error) {
							return "x", token, nil
						}),
					),
				},
			}, nil
		}
		return docker.ConfigureDefaultRegistries()(host)
	}
}
