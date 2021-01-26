package docker

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/superfly/flyctl/builtinsupport"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/briandowns/spinner"
	"github.com/buildpacks/pack"
	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
)

// ErrNoDockerfile - No dockerfile or builder or builtin or image specified error
var ErrNoDockerfile = errors.New("Project does not contain a Dockerfile and has not set a CNB builder, builtin builder or selected an image")

// ErrDockerDaemon - Docker daemon needs to be running error
var ErrDockerDaemon = errors.New("Docker daemon must be running to perform this action")

// ErrNoBuildpackBuilder - Unable to find Buildpack builder
var ErrNoBuildpackBuilder = errors.New("No buildpack builder")

// ResolveDockerfile - Resolve the location of the dockerfile, allowing for upper and lowercase naming
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

// Image - A type to hold information about a Docker image, including ID, Tag and Size
type Image struct {
	ID   string
	Tag  string
	Size int64
}

// BuildWithDocker - Run a Docker Build operation reporting back via the command context
func (op *DeployOperation) BuildWithDocker(cmdCtx *cmdctx.CmdContext, dockerfilePath string, buildArgs map[string]string) (*Image, error) {
	spinning := cmdCtx.OutputJSON()
	cwd := cmdCtx.WorkingDir
	appConfig := cmdCtx.AppConfig

	if !op.DockerAvailable() {
		return nil, ErrDockerDaemon
	}

	if dockerfilePath == "" {
		dockerfilePath = ResolveDockerfile(cwd)
	}

	if dockerfilePath == "" && !appConfig.HasBuiltin() {
		return nil, ErrNoDockerfile
	}

	if appConfig.HasBuiltin() {
		cmdCtx.Statusf("build", cmdctx.SDETAIL, "Using Builtin Builder: %s\n", appConfig.Build.Builtin)
	} else {
		cmdCtx.Statusf("build", cmdctx.SDETAIL, "Using Dockerfile Builder: %s\n", dockerfilePath)
	}

	buildContext, err := newBuildContext()
	if err != nil {
		return nil, err
	}
	defer buildContext.Close()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	if spinning {
		s.Writer = os.Stderr
		s.Prefix = "Creating build context... "
		s.Start()
	}

	excludes, err := readDockerignore(cwd)
	if err != nil {
		return nil, err
	}
	excludes = append(excludes, "fly.toml")

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
	} else {
		// We're doing a builtin!
		builtin, err := builtinsupport.GetBuiltin(cmdCtx, appConfig.Build.Builtin)
		if err != nil {
			return nil, err
		}
		// Expand args
		vdockerfile, err := builtin.GetVDockerfile(appConfig.Build.Settings)
		if err != nil {
			return nil, err
		}
		if err := buildContext.AddFile("Dockerfile", strings.NewReader(vdockerfile)); err != nil {
			return nil, err
		}
	}

	if spinning {
		s.Stop()
	}

	archive, err := buildContext.Archive()
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	normalizedBuildArgs := normalizeBuildArgs(appConfig, buildArgs)

	img, err := op.dockerClient.BuildImage(op.ctx, archive.File, op.imageTag, normalizedBuildArgs, op.out)

	if err != nil {
		return nil, err
	}

	image := &Image{
		ID:   img.ID,
		Tag:  op.imageTag,
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

// BuildWithPack - Perform a Docker build using a Buildpack (buildpack.io)
func (op *DeployOperation) BuildWithPack(cmdCtx *cmdctx.CmdContext, buildArgs map[string]string) (*Image, error) {
	cwd := cmdCtx.WorkingDir
	appConfig := cmdCtx.AppConfig

	if !op.DockerAvailable() {
		return nil, ErrDockerDaemon
	}

	if appConfig.Build == nil || appConfig.Build.Builder == "" {
		return nil, ErrNoBuildpackBuilder
	}

	c := initPackClient()

	env := map[string]string{}

	for name, val := range appConfig.Build.Args {
		env[name] = fmt.Sprint(val)
	}
	for name, val := range buildArgs {
		env[name] = val
	}

	err := c.Build(op.ctx, pack.BuildOptions{
		AppPath:    cwd,
		Builder:    appConfig.Build.Builder,
		Image:      op.imageTag,
		Buildpacks: appConfig.Build.Buildpacks,
		Env:        env,
	})

	if err != nil {
		return nil, err
	}

	cmdCtx.Status("build", cmdctx.SINFO, "Image built", op.imageTag)

	img, err := op.dockerClient.findImage(op.ctx, op.imageTag)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	image := &Image{
		ID:   img.ID,
		Tag:  op.imageTag,
		Size: img.Size,
	}

	return image, nil
}

// PushImage - Push the Image (where?)
func (op *DeployOperation) PushImage(image Image) error {
	return op.pushImage(image.Tag)
}

// StartRemoteBuild - Start a remote build and track its progress
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
