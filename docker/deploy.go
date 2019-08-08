package docker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
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

	op.checkDocker()

	return op, nil
}

func (op *DeployOperation) checkDocker() {
	if err := op.dockerClient.Check(); err != nil {
		terminal.Warn("Error connecting to Docker, only public images can be deployed without docker:\n", err)
		op.dockerAvailable = false
		return
	}
	op.dockerAvailable = true
}

func (op *DeployOperation) Deploy(imageRef string) (*api.Release, error) {
	if !op.dockerAvailable {
		return op.deployImageWithoutDocker(imageRef)
		if isRemoteImageReference(imageRef) {
			return op.deployImage(imageRef)
		}
		return nil, fmt.Errorf("Cannot deploy '%s' without the docker daemon running", imageRef)
	}

	var deploymentTag string

	buildDir, err := resolveBuildPath(imageRef)
	if err != nil {
		return nil, err
	}
	if buildDir != "" {
		deploymentTag, err = op.buildLocalPath(buildDir)
	} else {
		deploymentTag, err = op.resolveAndTagImageRef(imageRef)
	}
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

	go op.cleanDeploymentTags()

	return release, nil
}

func (op *DeployOperation) deployImageWithoutDocker(imageRef string) (*api.Release, error) {
	ref, err := checkManifest(imageRef, "")
	if err != nil {
		return nil, err
	}

	return op.deployImage(ref.Repository())
}

func (op *DeployOperation) buildLocalPath(path string) (string, error) {
	printHeader("Building image")

	tag := NewDeploymentTag(op.AppName)

	buildContext, err := NewBuildContext(path, tag)
	if err != nil {
		return "", err
	}

	if err := op.dockerClient.BuildImage(buildContext, op.out); err != nil {
		return "", err
	}

	return tag, nil
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

	imageTag := NewDeploymentTag(op.AppName)

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

var remotePattern = regexp.MustCompile(`^[\w\d-]+\.`)

func isRemoteImageReference(imageName string) bool {
	if strings.HasPrefix(imageName, ".") {
		return false
	}

	return remotePattern.MatchString(imageName)
}

func isDockerfilePath(imageName string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	maybePath := path.Join(cwd, imageName)

	return helpers.FileExists(maybePath)
}

func isDirContainingDockerfile(imageName string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	maybePath := path.Join(cwd, imageName, "Dockerfile")

	return helpers.FileExists(maybePath)
}

func resolveBuildPath(imageRef string) (string, error) {
	if isDockerfilePath(imageRef) {
		fmt.Printf("found file at '%s'\n", imageRef)
		return path.Dir(imageRef), nil
	} else if isDirContainingDockerfile(imageRef) {
		fmt.Printf("found Dockerfile in '%s'\n", imageRef)
		return imageRef, nil
	} else if strings.HasPrefix(imageRef, ".") {
		fmt.Printf("'%s' is a local path\n", imageRef)
		return filepath.Abs(imageRef)
	}

	return "", nil
}

func printHeader(message string) {
	fmt.Println(aurora.Blue("==>"), message)
}
