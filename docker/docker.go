package docker

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/mholt/archiver"
	dockerparser "github.com/novln/docker-parser"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
)

func NewDeploymentTag(appName string) string {
	t := time.Now()

	return fmt.Sprintf("%s%d", DeploymentTagPrefix(appName), t.Unix())
}

func DeploymentTagPrefix(appName string) string {
	return fmt.Sprintf("registry.fly.io/%s:deployment-", appName)
}

type DockerClient struct {
	ctx          context.Context
	docker       *client.Client
	registryAuth string
}

func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation(), client.WithVersion("1.40"))
	if err != nil {
		return nil, err
	}

	if err := client.FromEnv(cli); err != nil {
		return nil, err
	}

	accessToken := viper.GetString(flyctl.ConfigAPIAccessToken)

	authConfig := types.AuthConfig{
		Username:      accessToken,
		Password:      "x",
		ServerAddress: "registry.fly.io",
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return nil, err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	c := &DockerClient{
		ctx:          context.Background(),
		docker:       cli,
		registryAuth: authStr,
	}

	return c, nil
}

func (c *DockerClient) Check() error {
	_, err := c.docker.Ping(c.ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *DockerClient) ResolveImage(imageName string) (*types.ImageSummary, error) {
	img, err := c.FindImage(imageName)
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

	if err := c.PullImage(ref.Remote(), os.Stdout); err != nil {
		return nil, err
	}

	return c.FindImage(imageName)
}

func (c *DockerClient) PullImage(imageName string, out io.Writer) error {
	resp, err := c.docker.ImagePull(c.ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)
	return jsonmessage.DisplayJSONMessagesStream(resp, out, termFd, isTerm, nil)
}

func (c *DockerClient) TagImage(sourceRef, tag string) error {
	return c.docker.ImageTag(c.ctx, sourceRef, tag)
}

func (c *DockerClient) DeleteDeploymentImages(appName string) error {
	tagPrefix := DeploymentTagPrefix(appName)

	filters := filters.NewArgs()
	filters.Add("reference", tagPrefix+"*")

	images, err := c.docker.ImageList(c.ctx, types.ImageListOptions{Filters: filters})
	if err != nil {
		return err
	}

	for _, image := range images {
		for _, tag := range image.RepoTags {
			_, err := c.docker.ImageRemove(c.ctx, tag, types.ImageRemoveOptions{PruneChildren: true})
			if err != nil {
				terminal.Error("Error deleting image", err)
			}
			fmt.Println("Removed deployment image:", tag)
		}
	}

	return nil
}

func (c *DockerClient) BuildImage(ctx *BuildContext, out io.Writer) error {
	go func() {
		defer ctx.Close()

		if err := ctx.Load(); err != nil {
			terminal.Error(err)
		}
	}()

	resp, err := c.docker.ImageBuild(c.ctx, ctx, types.ImageBuildOptions{
		Tags:      []string{ctx.Tag},
		BuildArgs: ctx.BuildArgs(),
		// NoCache:   true,
	})

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)

	return jsonmessage.DisplayJSONMessagesStream(resp.Body, out, termFd, isTerm, nil)
}

func tarBuildContext(writer io.Writer, path string) error {
	tar := archiver.Tar{
		MkdirAll: true,
	}

	if err := tar.Create(writer); err != nil {
		return err
	}

	err := filepath.Walk(path, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(path, fpath)
		if err != nil {
			return err
		}

		file, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer file.Close()

		err = tar.Write(archiver.File{
			FileInfo: archiver.FileInfo{
				FileInfo:   info,
				CustomName: relPath,
			},
			ReadCloser: file,
		})

		return err
	})

	return err
}

var imageIDPattern = regexp.MustCompile("[a-f0-9]")

func (c *DockerClient) FindImage(imageName string) (*types.ImageSummary, error) {
	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		return nil, err
	}

	isID := imageIDPattern.MatchString(imageName)

	images, err := c.docker.ImageList(c.ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	if isID {
		for _, img := range images {
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

func (c *DockerClient) PushImage(imageName string, out io.Writer) error {
	resp, err := c.docker.ImagePush(c.ctx, imageName, types.ImagePushOptions{RegistryAuth: c.registryAuth})
	if err != nil {
		return err
	}
	defer resp.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)
	return jsonmessage.DisplayJSONMessagesStream(resp, out, termFd, isTerm, nil)
}

func checkManifest(imageRef string, token string) (*dockerparser.Reference, error) {
	ref, err := dockerparser.Parse(imageRef)
	if err != nil {
		return nil, err
	}

	registry := ref.Registry()
	if registry == "docker.io" {
		registry = "registry-1.docker.io"
	}
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, ref.ShortName(), ref.Tag())

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return ref, nil
	}

	if resp.StatusCode == 401 && ref.Registry() == "docker.io" && token == "" {
		token, _ := getDockerHubToken(ref.ShortName())
		if token != "" {
			return checkManifest(imageRef, token)
		}
	}

	return nil, fmt.Errorf("Unable to access image %s: %s", imageRef, resp.Status)
}

func getDockerHubToken(imageName string) (string, error) {
	url := fmt.Sprintf("https://auth.docker.io/token?scope=repository:%s:pull&service=registry.docker.io", imageName)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", errors.New("Unable to fetch registry token")
	}

	defer resp.Body.Close()

	data := map[string]string{}

	json.NewDecoder(resp.Body).Decode(&data)

	return data["token"], nil
}
