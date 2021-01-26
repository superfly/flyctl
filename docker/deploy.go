package docker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/dustin/go-humanize"
	"github.com/jpillora/backoff"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
	"golang.org/x/net/http/httpproxy"
)

type DeployOperation struct {
	ctx             context.Context
	dockerClient    *DockerClient
	apiClient       *api.Client
	dockerAvailable bool
	out             io.Writer
	appName         string
	appConfig       *flyctl.AppConfig
	imageTag        string
	remoteOnly      bool
	localOnly       bool
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
		Min:    250 * time.Millisecond,
		Max:    5 * time.Second,
		Factor: 1.5,
		Jitter: true,
	}

	noErrsInARow := 0

OUTER:
	for {
		checkErr := make(chan error, 1)

		go func() {
			checkErr <- dockerClient.Check(ctx)
		}()

		select {
		case err := <-checkErr:
			if err == nil {
				noErrsInARow++
				if noErrsInARow >= 3 {
					terminal.Info("Remote builder is ready to build!")
					break OUTER
				}
				b.Reset()
				dur := b.Duration()
				terminal.Debugf("Remote builder available, but pinging again in %s to be sure\n", dur)
				time.Sleep(dur)
			} else {
				noErrsInARow = 0
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

func NewDeployOperation(ctx context.Context, cmdCtx *cmdctx.CmdContext) (*DeployOperation, error) {
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

	op := &DeployOperation{
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

	if err := op.dockerClient.Check(ctx); err == nil {
		op.dockerAvailable = true
	} else {
		terminal.Debugf("Error pinging docker: %s\n", err)
	}

	return op, nil
}

func (op *DeployOperation) AppName() string {
	if op.appName != "" {
		return op.appName
	}
	return op.appConfig.AppName
}

func (op *DeployOperation) DockerAvailable() bool {
	return op.dockerAvailable
}

func (op *DeployOperation) LocalOnly() bool {
	return op.localOnly
}

func (op *DeployOperation) RemoteOnly() bool {
	return op.remoteOnly
}

type DeploymentStrategy string

const (
	CanaryDeploymentStrategy    DeploymentStrategy = "canary"
	RollingDeploymentStrategy   DeploymentStrategy = "rolling"
	ImmediateDeploymentStrategy DeploymentStrategy = "immediate"
	DefaultDeploymentStrategy   DeploymentStrategy = ""
)

func ParseDeploymentStrategy(val string) (DeploymentStrategy, error) {
	switch val {
	case "canary":
		return CanaryDeploymentStrategy, nil
	case "rolling":
		return RollingDeploymentStrategy, nil
	case "immediate":
		return ImmediateDeploymentStrategy, nil
	default:
		return "", fmt.Errorf("Unknown deployment strategy '%s'", val)
	}
}

func (op *DeployOperation) ValidateConfig() (*api.AppConfig, error) {
	if op.appConfig == nil {
		op.appConfig = flyctl.NewAppConfig()
	}

	parsedConfig, err := op.apiClient.ParseConfig(op.appName, op.appConfig.Definition)
	if err != nil {
		return parsedConfig, err
	}

	if !parsedConfig.Valid {
		return parsedConfig, errors.New("App configuration is not valid")
	}

	op.appConfig.Definition = parsedConfig.Definition

	return parsedConfig, nil
}

func (op *DeployOperation) ResolveImageLocally(ctx context.Context, cmdCtx *cmdctx.CmdContext, imageRef string) (*Image, error) {
	cmdCtx.Status("deploy", "Resolving image")

	if !op.DockerAvailable() || op.RemoteOnly() {
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

func (op *DeployOperation) pushImage(imageTag string) error {

	if imageTag == "" {
		return errors.New("invalid image reference")
	}

	if err := op.dockerClient.PushImage(op.ctx, imageTag, op.out); err != nil {
		return err
	}

	return nil
}

func (op *DeployOperation) Deploy(imageRef string, strategy DeploymentStrategy) (*api.Release, error) {
	return op.deployImage(imageRef, strategy)
}

func (op *DeployOperation) deployImage(imageTag string, strategy DeploymentStrategy) (*api.Release, error) {
	input := api.DeployImageInput{AppID: op.AppName(), Image: imageTag}
	if strategy != DefaultDeploymentStrategy {
		input.Strategy = api.StringPointer(strings.ToUpper(string(strategy)))
	}

	if op.appConfig != nil && len(op.appConfig.Definition) > 0 {
		x := api.Definition(op.appConfig.Definition)
		input.Definition = &x
	}

	release, err := op.apiClient.DeployImage(input)
	if err != nil {
		return nil, err
	}
	return release, err
}

func (op *DeployOperation) CleanDeploymentTags() {
	if !op.dockerAvailable {
		return
	}
	err := op.dockerClient.DeleteDeploymentImages(op.ctx, op.imageTag)
	if err != nil {
		terminal.Debugf("Error cleaning deployment tags: %s", err)
	}
}
