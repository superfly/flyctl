package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/console"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stringid"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/moby/term"
	"github.com/pkg/errors"

	dockerparser "github.com/novln/docker-parser"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
	"golang.org/x/net/http/httpproxy"
	"golang.org/x/sync/errgroup"
)

func newDeploymentTag(appName string, label string) string {
	if tag := os.Getenv("FLY_IMAGE_REF"); tag != "" {
		return tag
	}

	if label == "" {
		label = fmt.Sprintf("deployment-%d", time.Now().Unix())
	}

	registry := viper.GetString(flyctl.ConfigRegistryHost)

	return fmt.Sprintf("%s/%s:%s", registry, appName, label)
}

type DockerClient struct {
	docker       *client.Client
	registryAuth string
}

func (c *DockerClient) Client() *client.Client {
	return c.docker
}

var defaultOpts []client.Opt = []client.Opt{client.WithAPIVersionNegotiation()}

func newDockerClient(ops ...client.Opt) (*client.Client, error) {
	ops = append(defaultOpts, ops...)
	cli, err := client.NewClientWithOpts(ops...)
	if err != nil {
		return nil, err
	}

	if err := client.FromEnv(cli); err != nil {
		return nil, err
	}

	dockerHTTPProxy := os.Getenv("DOCKER_HTTP_PROXY")
	if dockerHTTPProxy != "" {
		t := cli.HTTPClient().Transport
		if t, ok := t.(*http.Transport); ok {
			cfg := &httpproxy.Config{HTTPProxy: dockerHTTPProxy}
			t.Proxy = func(req *http.Request) (*url.URL, error) {
				return cfg.ProxyFunc()(req.URL)
			}
		}
	}

	return cli, nil
}

func NewDockerClient() (*DockerClient, error) {
	cli, err := newDockerClient()
	if err != nil {
		return nil, err
	}

	accessToken := flyctl.GetAPIToken()
	authConfig := RegistryAuth(accessToken)
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return nil, err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)
	c := &DockerClient{
		docker:       cli,
		registryAuth: authStr,
	}

	return c, nil
}

func (c *DockerClient) Check(ctx context.Context) error {
	_, err := c.docker.Ping(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *DockerClient) Info(ctx context.Context) (types.Info, error) {
	return c.docker.Info(ctx)
}

func (c *DockerClient) ResolveImage(ctx context.Context, imageName string) (*types.ImageSummary, error) {
	img, err := c.findImage(ctx, imageName)
	if img != nil {
		return img, nil
	} else if err != nil {
		return nil, err
	}

	fmt.Printf("Unable to find image '%s' locally\n", imageName)

	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		return nil, err
	}

	if err := c.PullImage(ctx, ref.Remote(), os.Stdout); err != nil {
		return nil, err
	}

	return c.findImage(ctx, imageName)
}

func (c *DockerClient) PullImage(ctx context.Context, imageName string, out io.Writer) error {
	resp, err := c.docker.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)
	return jsonmessage.DisplayJSONMessagesStream(resp, out, termFd, isTerm, nil)
}

func (c *DockerClient) TagImage(ctx context.Context, sourceRef, tag string) error {
	return c.docker.ImageTag(ctx, sourceRef, tag)
}

func (c *DockerClient) DeleteDeploymentImages(ctx context.Context, tag string) error {
	filters := filters.NewArgs()
	filters.Add("reference", tag)

	images, err := c.docker.ImageList(ctx, types.ImageListOptions{Filters: filters})
	if err != nil {
		return err
	}

	for _, image := range images {
		for _, tag := range image.RepoTags {
			_, err := c.docker.ImageRemove(ctx, tag, types.ImageRemoveOptions{PruneChildren: true})
			if err != nil {
				terminal.Debug("Error deleting image", err)
			}
		}
	}

	return nil
}

func (c *DockerClient) buildkitEnabled() (buildkitEnabled bool, err error) {
	ping, err := c.docker.Ping(context.Background())
	if err != nil {
		return false, err
	}

	buildkitEnabled = ping.BuilderVersion == types.BuilderBuildKit
	if buildkitEnv := os.Getenv("DOCKER_BUILDKIT"); buildkitEnv != "" {
		buildkitEnabled, err = strconv.ParseBool(buildkitEnv)
		if err != nil {
			return false, errors.Wrap(err, "DOCKER_BUILDKIT environment variable expects boolean value")
		}
	}
	return buildkitEnabled, nil
}

func (c *DockerClient) BuildImage(ctx context.Context, contextDir string, tar io.Reader, tag string, buildArgs map[string]*string, out io.Writer) (*types.ImageSummary, error) {
	buildkitEnabled, err := c.buildkitEnabled()
	if err != nil {
		return nil, err
	}

	if buildkitEnabled {
		return c.doBuildKitBuild(ctx, contextDir, tar, tag, buildArgs, out)
	}

	opts := types.ImageBuildOptions{
		Tags:      []string{tag},
		BuildArgs: buildArgs,
		// NoCache:   true,
		AuthConfigs: authConfigs(),
		Platform:    "linux/amd64",
	}

	resp, err := c.docker.ImageBuild(ctx, tar, opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, out, termFd, isTerm, nil); err != nil {
		return nil, err
	}

	return c.findImage(ctx, tag)
}

const uploadRequestRemote = "upload-request"

func (c *DockerClient) doBuildKitBuild(ctx context.Context, contextDir string, tar io.Reader, tag string, buildArgs map[string]*string, out io.Writer) (*types.ImageSummary, error) {
	s, err := createBuildSession(contextDir)
	if err != nil {
		panic(err)
	}

	if s == nil {
		panic("buildkit not supported")
	}

	eg, errCtx := errgroup.WithContext(ctx)

	dialSession := func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
		return c.docker.DialHijack(errCtx, "/session", proto, meta)
	}
	eg.Go(func() error {
		return s.Run(context.TODO(), dialSession)
	})

	buildID := stringid.GenerateRandomID()
	eg.Go(func() error {
		buildOptions := types.ImageBuildOptions{
			Version: types.BuilderBuildKit,
			BuildID: uploadRequestRemote + ":" + buildID,
		}

		response, err := c.docker.ImageBuild(context.Background(), tar, buildOptions)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		return nil
	})

	eg.Go(func() error {
		defer s.Close()

		opts := types.ImageBuildOptions{
			Tags:      []string{tag},
			BuildArgs: buildArgs,
			// NoCache:   true,
			Version:       types.BuilderBuildKit,
			AuthConfigs:   authConfigs(),
			SessionID:     s.ID(),
			RemoteContext: uploadRequestRemote,
			BuildID:       buildID,
		}

		return doBuildKitBuild(errCtx, c.docker, eg, tar, opts)
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return c.findImage(ctx, tag)
}

func doBuildKitBuild(ctx context.Context, dockerClient *client.Client, eg *errgroup.Group, tar io.Reader, buildOptions types.ImageBuildOptions) error {
	resp, err := dockerClient.ImageBuild(ctx, nil, buildOptions)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	done := make(chan struct{})
	defer close(done)

	eg.Go(func() error {
		select {
		case <-ctx.Done():
			return dockerClient.BuildCancel(context.TODO(), buildOptions.BuildID)
		case <-done:
		}
		return nil
	})

	termFd, isTerm := term.GetFdInfo(os.Stderr)
	tracer := newTracer()
	var c2 console.Console
	if isTerm {
		if cons, err := console.ConsoleFromFile(os.Stderr); err == nil {
			c2 = cons
		}
	}

	eg.Go(func() error {
		return progressui.DisplaySolveStatus(context.TODO(), "", c2, os.Stderr, tracer.displayCh)
	})

	auxCallback := func(m jsonmessage.JSONMessage) {
		// if m.ID == "moby.image.id" {
		// 	var result types.BuildResult
		// 	if err := json.Unmarshal(*m.Aux, &result); err != nil {
		// 		fmt.Fprintf(dockerCli.Err(), "failed to parse aux message: %v", err)
		// 	}
		// 	imageID = result.ID
		// 	return
		// }

		tracer.write(m)
	}
	defer close(tracer.displayCh)

	buf := bytes.NewBuffer(nil)

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, buf, termFd, isTerm, auxCallback); err != nil {
		return err
	}

	return nil
}

var imageIDPattern = regexp.MustCompile("[a-f0-9]")

func (c *DockerClient) findImage(ctx context.Context, imageName string) (*types.ImageSummary, error) {
	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		return nil, err
	}

	isID := imageIDPattern.MatchString(imageName)

	images, err := c.docker.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	if isID {
		for _, img := range images {
			if len(img.ID) < len(imageName)+7 {
				continue
			}
			if img.ID[7:7+len(imageName)] == imageName {
				terminal.Debug("Found image by id", imageName)
				return &img, nil
			}
		}
	}

	searchTerms := []string{
		imageName,
		imageName + ":" + ref.Tag(),
		ref.Name(),
		ref.ShortName(),
		ref.Remote(),
		ref.Repository(),
	}

	terminal.Debug("Search terms:", searchTerms)

	for _, img := range images {
		for _, tag := range img.RepoTags {
			// skip <none>:<none>
			if strings.HasPrefix(tag, "<none>") {
				continue
			}

			for _, term := range searchTerms {
				if tag == term {
					return &img, nil
				}
			}
		}
	}

	return nil, nil
}

func (c *DockerClient) PushImage(ctx context.Context, imageName string, out io.Writer) error {
	resp, err := c.docker.ImagePush(ctx, imageName, types.ImagePushOptions{RegistryAuth: c.registryAuth})
	if err != nil {
		return err
	}
	defer resp.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)
	return jsonmessage.DisplayJSONMessagesStream(resp, out, termFd, isTerm, nil)
}

func RegistryAuth(token string) types.AuthConfig {
	return types.AuthConfig{
		Username:      "x",
		Password:      token,
		ServerAddress: "registry.fly.io",
	}
}

func authConfigs() map[string]types.AuthConfig {
	authConfigs := map[string]types.AuthConfig{}

	dockerhubUsername := os.Getenv("DOCKER_HUB_USERNAME")
	dockerhubPassword := os.Getenv("DOCKER_HUB_PASSWORD")

	if dockerhubUsername != "" && dockerhubPassword != "" {
		cfg := types.AuthConfig{
			Username:      dockerhubUsername,
			Password:      dockerhubPassword,
			ServerAddress: "index.docker.io",
		}
		authConfigs["https://index.docker.io/v1/"] = cfg
	}

	return authConfigs
}
