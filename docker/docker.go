package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	cli, err := client.NewEnvClient()
	if err != nil {
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
	// tarReader, tarWriter := io.Pipe()

	// ctx, err := newBuildContext(tarWriter)
	// if err != nil {
	// 	return err
	// }

	// builders, err := NewBuilderRepo()
	// if err != nil {
	// 	return err
	// }

	// fmt.Println(options.Manifest)

	// builder := options.Manifest.Builder()

	// if builder != "" {
	// 	if err := builders.Sync(); err != nil {
	// 		return err
	// 	}
	// }

	go func() {
		defer ctx.Close()

		if err := ctx.Load(); err != nil {
			panic(err)
			terminal.Error(err)
		}

		// if builder != "" {
		// 	fmt.Fprintln(out, "Build configuration detected, refreshing builders")

		// 	fmt.Fprintf(out, "Loading resources for builder %s\n", builder)
		// 	builderResourcesPath, err := builders.GetResourcesPath(builder)
		// 	if err != nil {
		// 		terminal.Error(err)
		// 		return
		// 	}

		// 	// add builder to context
		// 	fmt.Println("adding builder resources", builderResourcesPath)
		// 	if err := ctx.AddAll(builderResourcesPath); err != nil {
		// 		// if error occures here log and bail, the build will fail when the stream is closed
		// 		terminal.Error(err)
		// 		return
		// 	}
		// }

		// if err := ctx.AddAll(options.SourceDir); err != nil {
		// 	// if error occures here log and bail, the build will fail when the stream is closed
		// 	terminal.Error(err)
		// 	return
		// }
	}()

	// fmt.Println("tag", options.Tag)

	resp, err := c.docker.ImageBuild(c.ctx, ctx, types.ImageBuildOptions{
		Tags:      []string{ctx.Tag},
		BuildArgs: ctx.BuildArgs(),
		NoCache:   true,
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
