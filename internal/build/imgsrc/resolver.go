package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

type ImageOptions struct {
	AppName         string
	WorkingDir      string
	DockerfilePath  string
	ImageRef        string
	BuildArgs       map[string]string
	ExtraBuildArgs  map[string]string
	BuildSecrets    map[string]string
	ImageLabel      string
	Publish         bool
	Tag             string
	Target          string
	NoCache         bool
	BuiltIn         string
	BuiltInSettings map[string]interface{}
	Builder         string
	Buildpacks      []string
}

type RefOptions struct {
	AppName    string
	WorkingDir string
	ImageRef   string
	ImageLabel string
	Publish    bool
	Tag        string
}

type DeploymentImage struct {
	ID   string
	Tag  string
	Size int64
}

type Resolver struct {
	dockerFactory *dockerClientFactory
	apiClient     *api.Client
}

// ResolveReference returns an Image give an reference using either the local docker daemon or remote registry
func (r *Resolver) ResolveReference(ctx context.Context, streams *iostreams.IOStreams, opts RefOptions) (img *DeploymentImage, err error) {
	strategies := []imageResolver{
		&localImageResolver{},
		&remoteImageResolver{flyApi: r.apiClient},
	}

	for _, s := range strategies {
		terminal.Debugf("Trying '%s' strategy\n", s.Name())
		img, err = s.Run(ctx, r.dockerFactory, streams, opts)
		terminal.Debugf("result image:%+v error:%v\n", img, err)
		if err != nil {
			return nil, err
		}
		if img != nil {
			return img, nil
		}
	}

	return nil, fmt.Errorf("could not find image \"%s\"", opts.ImageRef)
}

// BuildImage converts source code to an image using a Dockerfile, buildpacks, or builtins.
func (r *Resolver) BuildImage(ctx context.Context, streams *iostreams.IOStreams, opts ImageOptions) (img *DeploymentImage, err error) {
	if !r.dockerFactory.mode.IsAvailable() {
		return nil, errors.New("docker is unavailable to build the deployment image")
	}

	if opts.Tag == "" {
		opts.Tag = NewDeploymentTag(opts.AppName, opts.ImageLabel)
	}

	strategies := []imageBuilder{}

	if r.dockerFactory.mode.UseNixpacks() {
		strategies = append(strategies, &nixpacksBuilder{})
	} else {
		strategies = []imageBuilder{
			&buildpacksBuilder{},
			&dockerfileBuilder{},
			&builtinBuilder{},
		}
	}

	for _, s := range strategies {
		terminal.Debugf("Trying '%s' strategy\n", s.Name())
		img, err = s.Run(ctx, r.dockerFactory, streams, opts)
		terminal.Debugf("result image:%+v error:%v\n", img, err)
		if err != nil {
			return nil, err
		}
		if img != nil {
			return img, nil
		}
	}

	return nil, errors.New("app does not have a Dockerfile or buildpacks configured. See https://fly.io/docs/reference/configuration/#the-build-section")
}

// For remote builders send a periodic heartbeat during build to ensure machine stays alive
// This is a noop for local builders
func (r *Resolver) StartHeartbeat(ctx context.Context) chan<- interface{} {
	if !r.dockerFactory.remote {
		return nil
	}

	errMsg := "Failed to start remote builder heartbeat. For builds longer than 10 minutes, this may cause issues. Please report issues to https://community.fly.io. %v"
	dockerClient, err := r.dockerFactory.buildFn(ctx)
	if err != nil {
		terminal.Warnf(errMsg, err)
		return nil
	}
	heartbeatUrl, err := getHeartbeatUrl(dockerClient)
	if err != nil {
		terminal.Warnf(errMsg, err)
		return nil
	}
	heartbeatReq, err := http.NewRequestWithContext(ctx, http.MethodGet, heartbeatUrl, http.NoBody)
	if err != nil {
		terminal.Warnf(errMsg, err)
		return nil
	}
	heartbeatReq.SetBasicAuth(r.dockerFactory.appName, flyctl.GetAPIToken())
	heartbeatReq.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Version().String()))

	pulseInterval := 30 * time.Second
	maxTime := 1 * time.Hour

	done := make(chan interface{})
	time.AfterFunc(maxTime, func() { close(done) })

	go func() {
		pulse := time.Tick(pulseInterval)
		defer close(done)
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-pulse:
				terminal.Debugf("Sending remote builder heartbeat pulse to %s...", heartbeatUrl)
				resp, err := dockerClient.HTTPClient().Do(heartbeatReq)
				if err != nil {
					terminal.Debugf("Remote builder heartbeat pulse failed: %v", err)
				} else {
					terminal.Debugf("Remote builder heartbeat response: %s", resp.Status)
				}
			}
		}
	}()
	return done
}

func getHeartbeatUrl(dockerClient *dockerclient.Client) (string, error) {
	daemonHost := dockerClient.DaemonHost()
	parsed, err := url.Parse(daemonHost)
	if err != nil {
		return "", err
	}
	hostPort := parsed.Host
	host, _, _ := net.SplitHostPort(hostPort)
	parsed.Host = host + ":8080"
	parsed.Scheme = "http"
	parsed.Path = "/flyio/v1/extendDeadline"
	return parsed.String(), nil
}

func (r *Resolver) StopHeartbeat(heartbeat chan<- interface{}) {
	heartbeat <- struct{}{}
}

func NewResolver(daemonType DockerDaemonType, apiClient *api.Client, appName string, iostreams *iostreams.IOStreams) *Resolver {
	return &Resolver{
		dockerFactory: newDockerClientFactory(daemonType, apiClient, appName, iostreams),
		apiClient:     apiClient,
	}
}

type imageBuilder interface {
	Name() string
	Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error)
}

type imageResolver interface {
	Name() string
	Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions) (*DeploymentImage, error)
}
