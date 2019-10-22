package docker

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
)

type DeployOperation struct {
	dockerClient    *DockerClient
	apiClient       *api.Client
	dockerAvailable bool
	out             io.Writer
	appName         string
	Project         *flyctl.Project
}

func NewDeployOperation(appName string, project *flyctl.Project, apiClient *api.Client, out io.Writer) (*DeployOperation, error) {
	dockerClient, err := NewDockerClient()
	if err != nil {
		return nil, err
	}

	op := &DeployOperation{
		dockerClient: dockerClient,
		apiClient:    apiClient,
		out:          out,
		appName:      appName,
		Project:      project,
	}

	op.dockerAvailable = op.dockerClient.Check() == nil

	return op, nil
}

func (op *DeployOperation) AppName() string {
	if op.appName != "" {
		return op.appName
	}
	return op.Project.AppName()
}

func (op *DeployOperation) DockerAvailable() bool {
	return op.dockerAvailable
}

func (op *DeployOperation) DeployImage(imageRef string) (*api.Release, error) {
	//if op.dockerAvailable {
	//	return op.deployImageWithDocker(imageRef)
	//}
	return op.deployImageWithoutDocker(imageRef)
}

func (op *DeployOperation) deployImageWithDocker(imageRef string) (*api.Release, error) {
	deploymentTag, err := op.resolveAndTagImageRef(imageRef)
	if err != nil {
		return nil, err
	}

	if err := op.pushImage(deploymentTag); err != nil {
		return nil, err
	}

	release, err := op.deployImage(deploymentTag)
	if err != nil {
		return nil, err
	}

	op.cleanDeploymentTags()

	return release, nil

}

func (op *DeployOperation) deployImageWithoutDocker(imageRef string) (*api.Release, error) {
	ref, err := checkManifest(imageRef, "")
	if err != nil {
		return nil, err
	}

	return op.deployImage(ref.Repository())
}

func (op *DeployOperation) resolveAndTagImageRef(imageRef string) (string, error) {
	printHeader("Resolving image")

	img, err := op.dockerClient.ResolveImage(imageRef)
	if err != nil {
		return "", err
	}

	if img == nil {
		return "", fmt.Errorf("Could not resolve image %s", imageRef)
	}

	fmt.Println("-->", img.ID)

	imageTag := newDeploymentTag(op.Project.AppName())

	printHeader("Creating deployment tag")
	if err := op.dockerClient.TagImage(img.ID, imageTag); err != nil {
		return "", err
	}
	fmt.Println("-->", imageTag)

	return imageTag, nil
}

func (op *DeployOperation) pushImage(imageTag string) error {
	printHeader("Pushing image")

	if imageTag == "" {
		return errors.New("invalid image reference")
	}

	if err := op.dockerClient.PushImage(imageTag, op.out); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	return nil
}

func (op *DeployOperation) deployImage(imageTag string) (*api.Release, error) {
	input := api.DeployImageInput{AppID: op.AppName(), Image: imageTag}

	if op.Project != nil {
		projectServices := op.Project.Services()

		if len(projectServices) > 0 {
			printHeader("Registering Services")

			services := op.Project.Services()

			for _, s := range services {
				handlers := "none"
				if len(s.Handlers) > 0 {
					handlers = strings.Join(s.Handlers, " ")
				}

				fmt.Printf("  %s %d --> %s %d (handlers: %s)\n", s.Protocol, s.Port, s.Protocol, s.InternalPort, handlers)
			}

			input.Services = &services
		}
	}

	printHeader("Creating Release")
	release, err := op.apiClient.DeployImage(input)
	if err != nil {
		return nil, err
	}
	return release, err
}

func (op *DeployOperation) cleanDeploymentTags() {
	err := op.dockerClient.DeleteDeploymentImages(op.AppName())
	if err != nil {
		terminal.Debug("Error cleaning deployment tags", err)
	}
}

func printHeader(message string) {
	fmt.Println(aurora.Blue("==>"), message)
}
