package docker

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/briandowns/spinner"
	"github.com/buildpacks/pack"
	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
)

var ErrNoDockerfile = errors.New("Project does not contain a Dockerfile or specify a builder")
var ErrDockerDaemon = errors.New("Docker daemon must be running to perform this action")
var ErrNoBuildpackBuilder = errors.New("No buildpack builder")

func ResolveDockerfile(cwd string) string {
	dockerfilePath := path.Join(cwd, "Dockerfile")
	if helpers.FileExists(dockerfilePath) {
		return dockerfilePath
	}
	dockerfilePath = path.Join(cwd, "dockerfile")
	if helpers.FileExists(dockerfilePath) {
		return dockerfilePath
	}
	return ""
}

type Image struct {
	ID   string
	Tag  string
	Size int64
}

func (op *DeployOperation) BuildWithDocker(cwd string, appConfig *flyctl.AppConfig, dockerfilePath string, buildArgs map[string]string) (*Image, error) {
	if !op.DockerAvailable() {
		return nil, ErrDockerDaemon
	}

	if dockerfilePath == "" {
		dockerfilePath = ResolveDockerfile(cwd)
	}

	if dockerfilePath == "" {
		return nil, ErrNoDockerfile
	}

	fmt.Println("Using Dockerfile:", dockerfilePath)

	buildContext, err := newBuildContext()
	if err != nil {
		return nil, err
	}
	defer buildContext.Close()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Creating build context... "
	s.Start()

	excludes, err := readDockerignore(cwd)
	if err != nil {
		return nil, err
	}
	excludes = append(excludes, "fly.toml")

	if err := buildContext.AddSource(cwd, excludes); err != nil {
		return nil, err
	}

	if err := buildContext.AddSource(dockerfilePath, nil); err != nil {
		return nil, err
	}

	s.Stop()

	archive, err := buildContext.Archive()
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	tag := newDeploymentTag(op.AppName())

	normalizedBuildArgs := normalizeBuildArgs(appConfig, buildArgs)

	img, err := op.dockerClient.BuildImage(op.ctx, archive.File, tag, normalizedBuildArgs, op.out, op.squash)

	if err != nil {
		return nil, err
	}

	image := &Image{
		ID:   img.ID,
		Tag:  tag,
		Size: img.Size,
	}

	return image, nil
}

func initPackClient() pack.Client {
	client, err := pack.NewClient()
	if err != nil {
		panic(err)
	}
	return *client
}

func (op *DeployOperation) BuildWithPack(cwd string, appConfig *flyctl.AppConfig) (*Image, error) {
	if !op.DockerAvailable() {
		return nil, ErrDockerDaemon
	}

	if appConfig.Build == nil || appConfig.Build.Builder == "" {
		return nil, ErrNoBuildpackBuilder
	}

	tag := newDeploymentTag(op.AppName())

	c := initPackClient()

	imageName := tag

	err := c.Build(op.ctx, pack.BuildOptions{
		AppPath:    cwd,
		Builder:    appConfig.Build.Builder,
		Image:      imageName,
		Buildpacks: appConfig.Build.Buildpacks,
	})

	if err != nil {
		return nil, err
	}

	fmt.Println("Image built", imageName)

	img, err := op.dockerClient.findImage(op.ctx, imageName)

	if err != nil {
		return nil, err
	}

	image := &Image{
		ID:   img.ID,
		Tag:  tag,
		Size: img.Size,
	}

	return image, nil
}

func (op *DeployOperation) PushImage(image Image) error {
	return op.pushImage(image.Tag)
}

func (op *DeployOperation) OptimizeImage(image Image) error {
	return op.optimizeImage(image.Tag)
}

func (op *DeployOperation) StartRemoteBuild(cwd string, appConfig *flyctl.AppConfig, dockerfilePath string, buildArgs map[string]string) (*api.Build, error) {
	buildContext, err := newBuildContext()
	if err != nil {
		return nil, err
	}
	defer buildContext.Close()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Creating build context... "
	s.Start()

	excludes, err := readGitignore(cwd)
	if err != nil {
		return nil, err
	}

	if err := buildContext.AddSource(cwd, excludes); err != nil {
		return nil, err
	}

	if dockerfilePath != "" {
		dockerfile, err := os.Open(dockerfilePath)
		if err != nil {
			return nil, err
		}
		defer dockerfile.Close()
		if err := buildContext.AddFile("Dockerfile", dockerfile); err != nil {
			return nil, err
		}
	}

	archive, err := buildContext.Archive()
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	s.Stop()

	s.Prefix = "Submitting build..."

	uploadFileName := fmt.Sprintf("source-%d.tar.gz", time.Now().Unix())
	getURL, putURL, err := op.apiClient.CreateSignedUrls(op.AppName(), uploadFileName)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PUT", putURL, archive.File)
	if err != nil {
		return nil, err
	}
	req.ContentLength = archive.Size

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("Error submitting build: %s", body)
	}

	input := api.StartBuildInput{
		AppID:      op.AppName(),
		SourceURL:  getURL,
		SourceType: "targz",
		BuildType:  api.StringPointer("flyctl_v1"),
	}

	for name, val := range buildArgs {
		input.BuildArgs = append(input.BuildArgs, api.BuildArgInput{Name: name, Value: val})
	}

	build, err := op.apiClient.StartBuild(input)
	if err != nil {
		return nil, err
	}
	s.Stop()

	return build, nil
}

func normalizeBuildArgs(appConfig *flyctl.AppConfig, extra map[string]string) map[string]*string {
	var out = map[string]*string{}

	if appConfig.Build != nil {
		for k, v := range appConfig.Build.Args {
			// docker needs a string pointer. since ranges reuse variables we need to deref a copy
			val := v
			out[k] = &val
		}
	}

	for name, value := range extra {
		// docker needs a string pointer. since ranges reuse variables we need to deref a copy
		val := value
		out[name] = &val
	}

	return out
}

func readDockerignore(workingDir string) ([]string, error) {
	file, err := os.Open(path.Join(workingDir, ".dockerignore"))
	if os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		terminal.Warn("Error reading dockerignore", err)
		return []string{}, nil
	}

	excludes, err := dockerignore.ReadAll(file)
	if err == nil {
		excludes = trimExcludes(excludes)
	}

	return excludes, err
}

func readGitignore(workingDir string) ([]string, error) {
	file, err := os.Open(path.Join(workingDir, ".gitignore"))
	if os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		terminal.Warn("Error reading gitignore", err)
		return []string{}, nil
	}

	excludes, err := dockerignore.ReadAll(file)
	if err == nil {
		excludes = trimExcludes(excludes)
	}

	return excludes, err
}

func trimExcludes(excludes []string) []string {
	if match, _ := fileutils.Matches(".dockerignore", excludes); match {
		excludes = append(excludes, "!.dockerignore")
	}

	if match, _ := fileutils.Matches("Dockerfile", excludes); match {
		excludes = append(excludes, "![Dd]ockerfile")
	}

	return excludes
}

func printImageSize(size uint64) {
	fmt.Println(aurora.Bold(fmt.Sprintf("Image size: %s", humanize.Bytes(size))))
}
