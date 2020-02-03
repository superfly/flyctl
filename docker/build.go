package docker

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
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
	"golang.org/x/net/context"
)

type DockerfileSource uint

const (
	BuilderDockerfile DockerfileSource = iota
	CwdDockerfile
	NoDockerfile
)

var ErrNoDockerfile = errors.New("Project does not contain a Dockerfile or specify a builder")
var ErrDockerDaemon = errors.New("Docker daemon must be running to perform this action")

func dockerfileSource(cwd string, appConfig *flyctl.AppConfig) DockerfileSource {
	if helpers.FileExists(path.Join(cwd, "Dockerfile")) {
		return CwdDockerfile
	}
	if appConfig.Build != nil && appConfig.Build.Builder != "" {
		return BuilderDockerfile
	}
	return NoDockerfile
}

func (op *DeployOperation) BuildAndDeploy(cwd string, appConfig *flyctl.AppConfig) (*api.Release, error) {
	if !op.DockerAvailable() {
		return nil, ErrDockerDaemon
	}

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

	if err := buildContext.AddSource(cwd, excludes); err != nil {
		return nil, err
	}

	s.Stop()

	switch dockerfileSource(cwd, appConfig) {
	case NoDockerfile:
		return nil, ErrNoDockerfile
	case CwdDockerfile:
		fmt.Println("Using Dockerfile from working directory:", path.Join(cwd, "Dockerfile"))
	case BuilderDockerfile:
		builder := appConfig.Build.Builder
		fmt.Println("Using builder:", builder)
		builderPath, err := fetchBuilder(builder, cwd)
		defer os.RemoveAll(builderPath)

		if err != nil {
			return nil, err
		}
		if err := buildContext.AddSource(builderPath, []string{}); err != nil {
			return nil, err
		}
	}

	archive, err := buildContext.Archive()
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	tag := newDeploymentTag(op.AppName())

	buildArgs := normalizeBuildArgs(appConfig)

	img, err := op.dockerClient.BuildImage(archive.File, tag, buildArgs, op.out, op.squash)

	if err != nil {
		return nil, err
	}

	printImageSize(uint64(img.Size))

	if err := op.pushImage(tag); err != nil {
		return nil, err
	}

	if err := op.optimizeImage(tag); err != nil {
		return nil, err
	}

	release, err := op.deployImage(tag)
	if err != nil {
		return nil, err
	}

	op.cleanDeploymentTags()

	return release, nil
}

func initPackClient() pack.Client {
	client, err := pack.NewClient()
	if err != nil {
		panic(err)
		// exitError(logger, err)
	}
	return *client
}

func createCancellableContext() context.Context {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-signals
		cancel()
	}()

	return ctx
}

func (op *DeployOperation) PackAndDeploy(cwd string, appConfig *flyctl.AppConfig, builder string, buildpacks []string) (*api.Release, error) {
	if !op.DockerAvailable() {
		return nil, ErrDockerDaemon
	}

	tag := newDeploymentTag(op.AppName())

	c := initPackClient()
	fmt.Println("client", c)

	cc := createCancellableContext()
	imageName := tag

	err := c.Build(cc, pack.BuildOptions{
		AppPath: cwd,
		Builder: builder,
		// AdditionalMirrors: getMirrors(cfg),
		// RunImage:          flags.RunImage,
		// Env:               env,
		Image: imageName,
		// Publish:           flags.Publish,
		// NoPull:            flags.NoPull,
		// ClearCache:        flags.ClearCache,
		Buildpacks: buildpacks,
		// ContainerConfig: pack.ContainerConfig{
		// 	Network: flags.Network,
		// },
	})

	if err != nil {
		return nil, err
	}

	fmt.Println("Image built", imageName)

	img, err := op.dockerClient.findImage(imageName)
	// if err != nil {
	// 	return nil, err
	// }

	// buildArgs := normalizeBuildArgs(appConfig)

	// img, err := op.dockerClient.BuildImage(archive.File, tag, buildArgs, op.out, op.squash)

	// if err != nil {
	// 	return nil, err
	// }

	printImageSize(uint64(img.Size))

	if err := op.pushImage(tag); err != nil {
		return nil, err
	}

	if err := op.optimizeImage(tag); err != nil {
		return nil, err
	}

	release, err := op.deployImage(tag)
	if err != nil {
		return nil, err
	}

	op.cleanDeploymentTags()

	return release, nil
}

func (op *DeployOperation) StartRemoteBuild(cwd string, appConfig *flyctl.AppConfig) (*api.Build, error) {
	if dockerfileSource(cwd, appConfig) == NoDockerfile {
		return nil, ErrNoDockerfile
	}

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

	build, err := op.apiClient.CreateBuild(op.AppName(), getURL, "targz")
	if err != nil {
		return nil, err
	}
	s.Stop()

	return build, nil
}

func normalizeBuildArgs(appConfig *flyctl.AppConfig) map[string]*string {
	var out = map[string]*string{}

	if appConfig.Build != nil {
		for k, v := range appConfig.Build.Args {
			k = strings.ToUpper(k)
			// docker needs a string pointer. since ranges reuse variables we need to deref a copy
			val := v
			out[k] = &val
		}
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
		excludes = append(excludes, "!Dockerfile")
	}

	return excludes
}

func printImageSize(size uint64) {
	fmt.Println(aurora.Bold(fmt.Sprintf("Image size: %s", humanize.Bytes(size))))
}
