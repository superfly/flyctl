package docker

import (
	"context"
	"net/http"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
)

type DockerClientMode int

const (
	Auto DockerClientMode = iota
	Local
	Remote
)

func NewClient(ctx context.Context, mode DockerClientMode, apiClient *api.Client, appName string) (*dockerclient.Client, error) {
	if mode == Auto || mode == Local {
		client, err := newLocalDockerClient()
		if err != nil {
			return nil, err
		}
		if dockerOK(ctx, client, 100*time.Millisecond) {
			return client, nil
		}
	}

	if mode == Auto || mode == Remote {
		client, err := newRemoteDockerClient(ctx, apiClient, appName)
		if err != nil {
			return nil, err
		}
		if dockerOK(ctx, client, 100*time.Millisecond) {
			return client, nil
		}
	}

	return nil, nil
}

func newLocalDockerClient(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
	ops = append(ops, dockerclient.WithAPIVersionNegotiation())
	c, err := dockerclient.NewClientWithOpts(ops...)
	if err != nil {
		return nil, err
	}

	if err := dockerclient.FromEnv(c); err != nil {
		return nil, err
	}

	return c, nil
}

func newRemoteDockerClient(ctx context.Context, apiClient *api.Client, appName string) (*dockerclient.Client, error) {
	host, err := remoteBuilderURL(apiClient, appName)
	if err != nil {
		return nil, err
	}

	terminal.Debugf("Remote Docker builder host: %s\n", host)

	httpc := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsconfig.ClientDefault(),
		},
	}

	client, err := dockerclient.NewClientWithOpts(
		dockerclient.WithAPIVersionNegotiation(),
		dockerclient.WithHTTPClient(httpc),
		dockerclient.WithHost(host),
		dockerclient.WithHTTPHeaders(map[string]string{
			"Authorization": basicAuth(appName, flyctl.GetAPIToken()),
		}))

	if err != nil {
		return nil, errors.Wrap(err, "Error creating docker client")
	}

	terminal.Infof("Waiting for remote builder to become available...\n")

	if err := WaitForDaemon(ctx, client); err != nil {
		return nil, err
	}

	return client, nil
}

func dockerOK(ctx context.Context, c *dockerclient.Client, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := c.Ping(ctx)
	return err == nil
}
