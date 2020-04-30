package docker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
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
}

func NewDeployOperation(ctx context.Context, appName string, appConfig *flyctl.AppConfig, apiClient *api.Client, out io.Writer, squash bool, remoteOnly bool, imageLabel string) (*DeployOperation, error) {
	dockerClient, err := NewDockerClient()
	if err != nil {
		return nil, err
	}

	op := &DeployOperation{
		ctx:          ctx,
		dockerClient: dockerClient,
		apiClient:    apiClient,
		out:          out,
		appName:      appName,
		appConfig:    appConfig,
		imageTag:     newDeploymentTag(appName, imageLabel),
	}

	op.dockerAvailable = !remoteOnly && op.dockerClient.Check(ctx) == nil

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

	printHeader("Validating app configuration")

	parsedConfig, err := op.apiClient.ParseConfig(op.appName, op.appConfig.Definition)
	if err != nil {
		return parsedConfig, err
	}

	if !parsedConfig.Valid {
		return parsedConfig, errors.New("App configuration is not valid")
	}

	op.appConfig.Definition = parsedConfig.Definition

	fmt.Println("-->", "done")

	return parsedConfig, nil
}

func (op *DeployOperation) ResolveImage(ctx context.Context, imageRef string) (*Image, error) {
	printHeader("Resolving image")

	if op.DockerAvailable() {
		imgSummary, err := op.dockerClient.findImage(ctx, imageRef)
		if err != nil {
			return nil, err
		}

		if imgSummary == nil {
			goto ResolveWithoutDocker
		}

		fmt.Printf("Image ID: %+v\n", imgSummary.ID)
		fmt.Println(aurora.Bold(fmt.Sprintf("Image size: %s", humanize.Bytes(uint64(imgSummary.Size)))))

		fmt.Println("--> done")

		printHeader("Creating deployment tag")
		if err := op.dockerClient.TagImage(op.ctx, imgSummary.ID, op.imageTag); err != nil {
			return nil, err
		}
		fmt.Println("-->", op.imageTag)

		image := &Image{
			ID:   imgSummary.ID,
			Size: imgSummary.Size,
			Tag:  op.imageTag,
		}

		if err := op.PushImage(*image); err != nil {
			return nil, err
		}

		return image, nil
	}

ResolveWithoutDocker:
	img, err := op.resolveImageWithoutDocker(ctx, imageRef)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("Could not find image '%s'", imageRef)
	}

	fmt.Println("-->", img.Tag)

	return img, nil
}

func (op *DeployOperation) resolveImageWithoutDocker(ctx context.Context, imageRef string) (*Image, error) {
	ref, err := CheckManifest(op.ctx, imageRef, "")
	if err != nil {
		return nil, err
	}

	image := Image{
		Tag: ref.Repository(),
	}

	return &image, nil
}

func (op *DeployOperation) pushImage(imageTag string) error {
	printHeader("Pushing image")

	if imageTag == "" {
		return errors.New("invalid image reference")
	}

	if err := op.dockerClient.PushImage(op.ctx, imageTag, op.out); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	return nil
}

func (op *DeployOperation) optimizeImage(imageTag string) error {
	printHeader("Optimizing image")
	defer fmt.Println("-->", "done")

	if isatty.IsTerminal(os.Stdout.Fd()) {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Writer = os.Stderr
		s.Prefix = "building fs... "
		s.Start()
		defer s.Stop()
	}

	delay := 0 * time.Second

	for {
		select {
		case <-time.After(delay):
			status, err := op.apiClient.OptimizeImage(op.AppName(), imageTag)
			if err != nil {
				return err
			}
			if status != "in_progress" {
				return nil
			}
			delay = 1 * time.Second
		case <-op.ctx.Done():
			return op.ctx.Err()
		}
	}
}

func (op *DeployOperation) Deploy(image Image, strategy DeploymentStrategy) (*api.Release, error) {
	return op.deployImage(image.Tag, strategy)
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

	printHeader("Creating Release")
	if strategy != DefaultDeploymentStrategy {
		fmt.Fprintln(op.out, "Deployment Strategy:", strategy)
	}
	release, err := op.apiClient.DeployImage(input)
	if err != nil {
		return nil, err
	}
	return release, err
}

func (op *DeployOperation) CleanDeploymentTags() {
	err := op.dockerClient.DeleteDeploymentImages(op.ctx, op.imageTag)
	if err != nil {
		terminal.Debug("Error cleaning deployment tags", err)
	}
}

func printHeader(message string) {
	fmt.Println(aurora.Blue("==>"), message)
}
