package docker

import (
	"errors"
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

type DeployOperation struct {
	dockerClient    *DockerClient
	apiClient       *api.Client
	dockerAvailable bool
	out             io.Writer
	AppName         string
}

func NewDeployOperation(appName string, apiClient *api.Client, out io.Writer) (*DeployOperation, error) {
	dockerClient, err := NewDockerClient()
	if err != nil {
		return nil, err
	}

	op := &DeployOperation{
		dockerClient: dockerClient,
		apiClient:    apiClient,
		AppName:      appName,
		out:          out,
	}

	op.dockerAvailable = op.dockerClient.Check() == nil

	return op, nil
}

func (op *DeployOperation) DockerAvailable() bool {
	return op.dockerAvailable
}

func (op *DeployOperation) DeployImage(imageRef string) (*api.Release, error) {
	if op.dockerAvailable {
		return op.deployImageWithDocker(imageRef)
	}
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

	imageTag := newDeploymentTag(op.AppName)

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
	printHeader("Deploying Image")
	release, err := op.apiClient.DeployImage(op.AppName, imageTag)
	if err != nil {
		return nil, err
	}
	fmt.Println("-->", "done")
	return release, err
}

func (op *DeployOperation) cleanDeploymentTags() {
	err := op.dockerClient.DeleteDeploymentImages(op.AppName)
	if err != nil {
		terminal.Debug("Error cleaning deployment tags", err)
	}
}

func printHeader(message string) {
	fmt.Println(aurora.Blue("==>"), message)
}
