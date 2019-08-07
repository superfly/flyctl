package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docker"
)

func newDeployCommand() *Command {
	cmd := BuildCommand(runDeploy, "deploy", "deploy a local image, remote image, or Dockerfile", os.Stdout, true, requireAppName)

	cmd.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runDeploy(ctx *CmdContext) error {

	imageRef := ctx.Args[0]

	dockerClient, err := docker.NewDockerClient()
	if err != nil {
		return err
	}

	imageTag := docker.NewDeploymentTag(ctx.AppName())

	printHeader("Resolving image")

	buildPath, err := resolveBuildPath(imageRef)
	if err != nil {
		return err
	}

	if buildPath != "" {
		printHeader("Building image")

		buildContext, err := docker.NewBuildContext(buildPath, imageTag)
		if err != nil {
			return err
		}

		if err := dockerClient.BuildImage(buildContext, ctx.Out); err != nil {
			return err
		}
	} else {

		img, err := dockerClient.ResolveImage(imageRef)
		if err != nil {
			return err
		}

		if img == nil {
			return fmt.Errorf("Could not resolve image %s", imageRef)
		}

		fmt.Println("-->", img.ID)

		printHeader("Creating deployment tag")
		if err := dockerClient.TagImage(img.ID, imageTag); err != nil {
			return err
		}
		fmt.Println("-->", imageTag)
	}

	printHeader("Pushing image")
	if err := dockerClient.PushImage(imageTag, ctx.Out); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	printHeader("Releasing")
	deployment, err := ctx.FlyClient.DeployImage(ctx.AppName(), imageTag)
	if err != nil {
		return err
	}
	fmt.Println("-->", "done")

	printHeader("Cleaning")
	if err := dockerClient.DeleteDeploymentImages(ctx.AppName()); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	if deployment.Status == "succeeded" {
		fmt.Printf("Deployment complete - v%d released\n", deployment.Release.Version)
	} else {
		fmt.Printf("Deployment failed - %s\n", deployment.Status)
	}

	return nil
}

func resolveBuildPath(imageRef string) (string, error) {
	if docker.IsDockerfilePath(imageRef) {
		fmt.Printf("found file at '%s'\n", imageRef)
		return path.Dir(imageRef), nil
	} else if docker.IsDirContainingDockerfile(imageRef) {
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
