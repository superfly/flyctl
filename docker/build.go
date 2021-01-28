package docker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jpillora/backoff"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/builtinsupport"
	"github.com/superfly/flyctl/cmdctx"
	"golang.org/x/net/context"
	"golang.org/x/net/http/httpproxy"

	"github.com/briandowns/spinner"
	"github.com/buildpacks/pack"
	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/fileutils"
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

type BuildOperation struct {
	ctx                  context.Context
	apiClient            *api.Client
	dockerClient         *DockerClient
	localDockerAvailable bool
	out                  io.Writer
	appName              string
	appConfig            *flyctl.AppConfig
	imageTag             string
	remoteOnly           bool
	localOnly            bool
}

func NewBuildOperation(ctx context.Context, cmdCtx *cmdctx.CmdContext) (*BuildOperation, error) {
	remoteOnly := cmdCtx.Config.GetBool("remote-only")
	localOnly := cmdCtx.Config.GetBool("local-only")

	if localOnly && remoteOnly {
		return nil, fmt.Errorf("Both --local-only and --remote-only are set - select only one")
	}

	imageLabel, _ := cmdCtx.Config.GetString("image-label")

	dockerClient, err := NewDockerClient()
	if err != nil {
		return nil, err
	}

	op := &BuildOperation{
		ctx:          ctx,
		dockerClient: dockerClient,
		apiClient:    cmdCtx.Client.API(),
		out:          cmdCtx.Out,
		appName:      cmdCtx.AppName,
		appConfig:    cmdCtx.AppConfig,
		imageTag:     newDeploymentTag(cmdCtx.AppName, imageLabel),
		localOnly:    localOnly,
		remoteOnly:   remoteOnly,
	}

	if err := op.dockerClient.Check(ctx); err == nil {
		op.localDockerAvailable = true
	} else {
		terminal.Debugf("Error pinging local docker: %s\n", err)
	}

	if remoteOnly {
		terminal.Info("Remote only, hooking you up with a remote Docker builder...")
		if err := setRemoteBuilder(ctx, cmdCtx, dockerClient); err != nil {
			return nil, err
		}
	} else if err := op.dockerClient.Check(ctx); err != nil {
		if localOnly {
			return nil, fmt.Errorf("Local docker unavailable and --local-only was passed, cannot proceed.")
		}
		terminal.Info("Local docker unavailable, hooking you up with a remote Docker builder...")
		if err := setRemoteBuilder(ctx, cmdCtx, dockerClient); err != nil {
			return nil, err
		}
	}

	return op, nil
}

func (op *BuildOperation) LocalDockerAvailable() bool {
	return op.localDockerAvailable
}

func (op *BuildOperation) LocalOnly() bool {
	return op.localOnly
}

func (op *BuildOperation) RemoteOnly() bool {
	return op.remoteOnly
}

func (op *BuildOperation) ResolveImageLocally(ctx context.Context, cmdCtx *cmdctx.CmdContext, imageRef string) (*Image, error) {
	cmdCtx.Status("deploy", "Resolving image")

	if !op.LocalDockerAvailable() || op.RemoteOnly() {
		return nil, nil
	}

	imgSummary, err := op.dockerClient.findImage(ctx, imageRef)
	if err != nil {
		return nil, err
	}

	if imgSummary == nil {
		return nil, nil
	}

	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image ID: %+v\n", imgSummary.ID)
	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image size: %s\n", humanize.Bytes(uint64(imgSummary.Size)))

	cmdCtx.Status("deploy", cmdctx.SDONE, "Image resolving done")

	cmdCtx.Status("deploy", cmdctx.SBEGIN, "Creating deployment tag")
	if err := op.dockerClient.TagImage(op.ctx, imgSummary.ID, op.imageTag); err != nil {
		return nil, err
	}
	cmdCtx.Status("deploy", cmdctx.SINFO, "-->", op.imageTag)

	image := &Image{
		ID:   imgSummary.ID,
		Size: imgSummary.Size,
		Tag:  op.imageTag,
	}

	err = op.PushImage(*image)

	if err != nil {
		return nil, err
	}

	return image, nil
}

func (op *BuildOperation) pushImage(imageTag string) error {

	if imageTag == "" {
		return errors.New("invalid image reference")
	}

	if err := op.dockerClient.PushImage(op.ctx, imageTag, op.out); err != nil {
		return err
	}

	return nil
}

func (op *BuildOperation) CleanDeploymentTags() {
	err := op.dockerClient.DeleteDeploymentImages(op.ctx, op.imageTag)
	if err != nil {
		terminal.Debugf("Error cleaning deployment tags: %s", err)
	}
}

// BuildWithDocker - Run a Docker Build operation reporting back via the command context
func (op *BuildOperation) BuildWithDocker(cmdCtx *cmdctx.CmdContext, dockerfilePath string, buildArgs map[string]string) (*Image, error) {
	spinning := cmdCtx.OutputJSON()
	cwd := cmdCtx.WorkingDir
	appConfig := cmdCtx.AppConfig

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

func (op *BuildOperation) initPackClient() pack.Client {
	client, err := pack.NewClient(pack.WithDockerClient(op.dockerClient.docker))
	if err != nil {
		panic(err)
	}
	return *client
}

// BuildWithPack - Perform a Docker build using a Buildpack (buildpack.io)
func (op *BuildOperation) BuildWithPack(cmdCtx *cmdctx.CmdContext, buildArgs map[string]string) (*Image, error) {
	cwd := cmdCtx.WorkingDir
	appConfig := cmdCtx.AppConfig

	if appConfig.Build == nil || appConfig.Build.Builder == "" {
		return nil, ErrNoBuildpackBuilder
	}

	c := op.initPackClient()

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
func (op *BuildOperation) PushImage(image Image) error {
	return op.pushImage(image.Tag)
}

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

func trimExcludes(excludes []string) []string {
	if match, _ := fileutils.Matches(".dockerignore", excludes); match {
		excludes = append(excludes, "!.dockerignore")
	}

	if match, _ := fileutils.Matches("Dockerfile", excludes); match {
		excludes = append(excludes, "![Dd]ockerfile")
	}

	return excludes
}

func setRemoteBuilder(ctx context.Context, cmdCtx *cmdctx.CmdContext, dockerClient *DockerClient) error {
	rawURL, release, err := cmdCtx.Client.API().EnsureRemoteBuilder(cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("could not create remote builder: %v", err)
	}

	terminal.Debugf("Remote Docker builder URL: %s\n", rawURL)
	terminal.Debugf("Remote Docker builder release: %+v\n", release)

	dockerClient.docker, err = newDockerClient(client.WithHost(fmt.Sprintf("tcp://%s", cmdCtx.AppName)))
	if err != nil {
		return fmt.Errorf("error resetting docker client to use remote builder config: %v", err)
	}

	dockerTransport, ok := dockerClient.docker.HTTPClient().Transport.(*http.Transport)
	if !ok {
		return fmt.Errorf("Docker client transport was not an HTTP transport, don't know what to do with that")
	}
	builderURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("Could not parse builder url '%s': %v", rawURL, err)
	}

	builderURL.User = url.UserPassword(cmdCtx.AppName, flyctl.GetAPIToken())

	proxyCfg := &httpproxy.Config{HTTPProxy: builderURL.String()}
	dockerTransport.Proxy = func(req *http.Request) (*url.URL, error) {
		return proxyCfg.ProxyFunc()(req.URL)
	}

	deadline := time.After(5 * time.Minute)

	terminal.Info("Waiting for remote builder to become available...")

	b := &backoff.Backoff{
		//These are the defaults
		Min:    200 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 1.2,
		Jitter: true,
	}

	consecutiveSuccesses := 0

OUTER:
	for {
		checkErr := make(chan error, 1)

		go func() {
			checkErr <- dockerClient.Check(ctx)
		}()

		select {
		case err := <-checkErr:
			if err == nil {
				if consecutiveSuccesses == 0 {
					// reset on the first success in a row so the next checks are a bit spaced out
					b.Reset()
				}
				consecutiveSuccesses++
				if consecutiveSuccesses >= 3 {
					terminal.Info("Remote builder is ready to build!")
					break OUTER
				}
				dur := b.Duration()
				terminal.Debugf("Remote builder available, but pinging again in %s to be sure\n", dur)
				time.Sleep(dur)
			} else {
				consecutiveSuccesses = 0
				dur := b.Duration()
				terminal.Debugf("Remote builder unavailable, retrying in %s (err: %v)\n", dur, err)
				time.Sleep(dur)
			}
		case <-deadline:
			return fmt.Errorf("Could not ping remote builder within 5 minutes, aborting.")
		case <-ctx.Done():
			terminal.Warn("Canceled")
			break OUTER
		}
	}

	return nil
}
