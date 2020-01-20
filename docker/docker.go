package docker

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	dockerparser "github.com/novln/docker-parser"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
)

func newDeploymentTag(appName string) string {
	t := time.Now()

	return fmt.Sprintf("%s%d", deploymentTagPrefix(appName), t.Unix())
}

func deploymentTagPrefix(appName string) string {
	registry := viper.GetString(flyctl.ConfigRegistryHost)
	return fmt.Sprintf("%s/%s:deployment-", registry, appName)
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

	accessToken := viper.GetString(flyctl.ConfigAPIToken)

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
	img, err := c.findImage(imageName)
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

	return c.findImage(imageName)
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
	tagPrefix := deploymentTagPrefix(appName)

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
				terminal.Debug("Error deleting image", err)
			}
		}
	}

	return nil
}

func (c *DockerClient) BuildImage(tar io.Reader, tag string, buildArgs map[string]*string, out io.Writer, squash bool) (*types.ImageSummary, error) {
	resp, err := c.docker.ImageBuild(c.ctx, tar, types.ImageBuildOptions{
		Tags:      []string{tag},
		BuildArgs: buildArgs,
		// NoCache:   true,
	})

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	termFd, isTerm := term.GetFdInfo(os.Stderr)

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, out, termFd, isTerm, nil); err != nil {
		return nil, err
	}

	img, err := c.findImage(tag)
	if err != nil {
		return nil, err
	}

	if !squash {
		return img, err
	}

	printHeader("Squashing image")

	fmt.Println("Creating temporary container")

	cont, err := c.docker.ContainerCreate(c.ctx, &container.Config{
		Image: img.ID,
	}, nil, nil, "")
	if err != nil {
		return nil, err
	}

	fmt.Println("Exporting rootfs")
	r, err := c.docker.ContainerExport(c.ctx, cont.ID)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	defer func(id string) {
		c.docker.ContainerRemove(c.ctx, id, types.ContainerRemoveOptions{})
	}(cont.ID)

	fmt.Println("Applying USER, ENTRYPOINT and CMD")

	contJSON, err := c.docker.ContainerInspect(c.ctx, cont.ID)
	if err != nil {
		return nil, err
	}

	entrypoint := []string{}
	for _, e := range contJSON.Config.Entrypoint {
		entrypoint = append(entrypoint, fmt.Sprintf("%q", e))
	}

	cmd := []string{}
	for _, c := range contJSON.Config.Cmd {
		cmd = append(cmd, fmt.Sprintf("%q", c))
	}

	fmt.Println("Importing flat image")
	j, err := c.docker.ImageImport(c.ctx, types.ImageImportSource{
		Source:     r,
		SourceName: "-",
	}, tag, types.ImageImportOptions{Changes: []string{
		fmt.Sprintf("ENTRYPOINT [%s]", strings.Join(entrypoint, ",")),
		fmt.Sprintf("CMD [%s]", strings.Join(cmd, ",")),
		fmt.Sprintf("USER %s", contJSON.Config.User),
	}})
	if err != nil {
		return nil, err
	}
	defer j.Close()

	// fmt.Println("RESP DECODE")
	// var respBody interface{}
	// err = json.NewDecoder(j).Decode(&respBody)
	// if err != nil {
	// 	return nil, err
	// }

	// fmt.Println("DEBUGGING RESP BODY", respBody)

	fmt.Println("--> done")

	return img, err
}

var imageIDPattern = regexp.MustCompile("[a-f0-9]")

func (c *DockerClient) findImage(imageName string) (*types.ImageSummary, error) {
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
