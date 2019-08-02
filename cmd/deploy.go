package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
)

func newDeployCommand() *cobra.Command {
	deploy := &pushCommand{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "deploy a local image, remote image, or Dockerfile",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return deploy.Init(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploy.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&deploy.appName, "app", "a", "", `app to run command against`)

	return cmd
}

type pushCommand struct {
	apiClient    *api.Client
	dockerClient *docker.DockerClient
	appName      string
	imageRef     string
	imageID      string
	imageTag     string
	deployment   api.Deployment
}

func (cmd *pushCommand) Init(args []string) error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	cmd.apiClient = client

	docker, err := docker.NewDockerClient()
	if err != nil {
		return err
	}
	cmd.dockerClient = docker

	if cmd.appName == "" {
		cmd.appName = flyctl.CurrentAppName()
	}
	if cmd.appName == "" {
		return fmt.Errorf("no app specified")
	}

	cmd.imageRef = args[0]

	return nil
}

func (cmd *pushCommand) Run(args []string) error {
	cmd.imageTag = docker.NewDeploymentTag(cmd.appName)

	cmdOutput := os.Stderr

	buildPath := ""

	printHeader("Resolving image")

	buildPath, err := cmd.resolveBuildPath()
	if err != nil {
		return err
	}

	if buildPath != "" {
		printHeader("Building image")
		if err := cmd.dockerClient.BuildImage(buildPath, cmd.imageTag, cmdOutput); err != nil {
			return err
		}
	} else {
		if err := cmd.locateImageID(); err != nil {
			return err
		}
		fmt.Println("-->", cmd.imageID)

		printHeader("Creating deployment tag")
		if err := cmd.dockerClient.TagImage(cmd.imageID, cmd.imageTag); err != nil {
			return err
		}
		fmt.Println("-->", cmd.imageTag)
	}

	printHeader("Pushing image")
	if err := cmd.dockerClient.PushImage(cmd.imageTag, cmdOutput); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	printHeader("Releasing")
	if err := cmd.deployImage(); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	printHeader("Cleaning")
	if err := cmd.dockerClient.DeleteDeploymentImages(cmd.appName); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	if cmd.deployment.Status == "succeeded" {
		fmt.Printf("Deployment complete - v%d released\n", cmd.deployment.Release.Version)
	} else {
		fmt.Printf("Deployment failed - %s\n", cmd.deployment.Status)
	}

	return nil
}

func (cmd *pushCommand) resolveBuildPath() (string, error) {
	if docker.IsDockerfilePath(cmd.imageRef) {
		fmt.Printf("found file at '%s'\n", cmd.imageRef)
		return path.Dir(cmd.imageRef), nil
	} else if docker.IsDirContainingDockerfile(cmd.imageRef) {
		fmt.Printf("found Dockerfile in '%s'\n", cmd.imageRef)
		return cmd.imageRef, nil
	} else if strings.HasPrefix(cmd.imageRef, ".") {
		fmt.Printf("'%s' is a local path\n", cmd.imageRef)
		return filepath.Abs(cmd.imageRef)
	}

	return "", nil
}

func (cmd *pushCommand) locateImageID() error {
	img, err := cmd.dockerClient.ResolveImage(cmd.imageRef)
	if err != nil {
		return err
	}

	if img == nil {
		return fmt.Errorf("Could not resolve image %s", cmd.imageRef)
	}

	cmd.imageID = img.ID

	return nil
}

func (cmd *pushCommand) deployImage() error {
	query := `
			mutation($input: DeployImageInput!) {
				deployImage(input: $input) {
					deployment {
						id
						status
						release {
							version
						}
					}
				}
			}
		`

	req := cmd.apiClient.NewRequest(query)

	req.Var("input", map[string]string{
		"appId": cmd.appName,
		"image": cmd.imageTag,
	})

	data, err := cmd.apiClient.Run(req)
	if err != nil {
		return err
	}

	cmd.deployment = data.DeployImage.Deployment

	return nil
}

func printHeader(message string) {
	fmt.Println(aurora.Blue("==>"), message)
}
