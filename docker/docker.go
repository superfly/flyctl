package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/docker/docker/pkg/term"
	"github.com/moby/buildkit/util/progress/progressui"
	dockerparser "github.com/novln/docker-parser"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"

	controlapi "github.com/moby/buildkit/api/services/control"
	buildkitClient "github.com/moby/buildkit/client"
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

func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	if err := client.FromEnv(cli); err != nil {
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

func (c *DockerClient) BuildImage(ctx context.Context, tar io.Reader, tag string, buildArgs map[string]*string, out io.Writer) (*types.ImageSummary, error) {
	buildkitEnabled, err := c.buildkitEnabled()
	if err != nil {
		return nil, err
	}
	if buildkitEnabled {
		return c.doBuildKitBuild(ctx, tar, tag, buildArgs, out)
	}

	opts := types.ImageBuildOptions{
		Tags:      []string{tag},
		BuildArgs: buildArgs,
		// NoCache:   true,
		AuthConfigs: authConfigs(),
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

func (c *DockerClient) doBuildKitBuild(ctx context.Context, tar io.Reader, tag string, buildArgs map[string]*string, out io.Writer) (*types.ImageSummary, error) {
	opts := types.ImageBuildOptions{
		Tags:      []string{tag},
		BuildArgs: buildArgs,
		// NoCache:   true,
		Version:     types.BuilderBuildKit,
		AuthConfigs: authConfigs(),
	}

	resp, err := c.docker.ImageBuild(ctx, tar, opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)

	tracer := newTracer()
	var c2 console.Console
	if isTerm {
		if cons, err := console.ConsoleFromFile(os.Stdout); err == nil {
			c2 = cons
		}
	}

	go func() {
		err := progressui.DisplaySolveStatus(context.TODO(), "", c2, out, tracer.displayCh)
		if err != nil {

			panic(err)
		}
	}()

	auxCallback := func(m jsonmessage.JSONMessage) {
		tracer.write(m)
	}

	buf := bytes.NewBuffer(nil)

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, buf, termFd, isTerm, auxCallback); err != nil {
		return nil, err
	}
	close(tracer.displayCh)

	return c.findImage(ctx, tag)
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

type tracer struct {
	displayCh chan *buildkitClient.SolveStatus
}

func newTracer() *tracer {
	return &tracer{
		displayCh: make(chan *buildkitClient.SolveStatus),
	}
}

func (t *tracer) write(msg jsonmessage.JSONMessage) {
	var resp controlapi.StatusResponse

	if msg.ID != "moby.buildkit.trace" {
		return
	}

	var dt []byte
	// ignoring all messages that are not understood
	if err := json.Unmarshal(*msg.Aux, &dt); err != nil {
		return
	}
	if err := (&resp).Unmarshal(dt); err != nil {
		return
	}

	s := buildkitClient.SolveStatus{}
	for _, v := range resp.Vertexes {
		s.Vertexes = append(s.Vertexes, &buildkitClient.Vertex{
			Digest:    v.Digest,
			Inputs:    v.Inputs,
			Name:      v.Name,
			Started:   v.Started,
			Completed: v.Completed,
			Error:     v.Error,
			Cached:    v.Cached,
		})
	}
	for _, v := range resp.Statuses {
		s.Statuses = append(s.Statuses, &buildkitClient.VertexStatus{
			ID:        v.ID,
			Vertex:    v.Vertex,
			Name:      v.Name,
			Total:     v.Total,
			Current:   v.Current,
			Timestamp: v.Timestamp,
			Started:   v.Started,
			Completed: v.Completed,
		})
	}
	for _, v := range resp.Logs {
		s.Logs = append(s.Logs, &buildkitClient.VertexLog{
			Vertex:    v.Vertex,
			Stream:    int(v.Stream),
			Data:      v.Msg,
			Timestamp: v.Timestamp,
		})
	}

	t.displayCh <- &s
}
