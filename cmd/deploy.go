package cmd

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/gosuri/uiprogress"
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

	buildPath := ""

	if docker.IsDockerfilePath(cmd.imageRef) {
		fmt.Println("we have a file, use the base dir and build!")
		buildPath = path.Dir(cmd.imageRef)
	} else if docker.IsDirContainingDockerfile(cmd.imageRef) {
		fmt.Println("we have a directory containing a dockerfile")
		buildPath = cmd.imageRef
	} else if strings.HasPrefix(cmd.imageRef, ".") {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		buildPath = cwd
	}

	if buildPath != "" {
		op := cmd.dockerClient.BuildImage(cmd.imageRef, cmd.imageTag)

		for msg := range op.Status() {
			fmt.Print(msg.Stream)
		}

		if op.Error() != nil {
			return op.Error()
		}
		// } else if docker.IsRemoteImageReference(cmd.imageRef) {
		// 	fmt.Println("it's a remote image, pull then deploy?")
		// 	return nil
	} else {
		fmt.Println("Locating image...")
		if err := cmd.locateImageID(); err != nil {
			return err
		}
		fmt.Println("-->", cmd.imageID)

		fmt.Println("Creating deployment tag...")
		if err := cmd.createDeploymentTag(); err != nil {
			return err
		}
		fmt.Println("-->", cmd.imageTag)
	}

	fmt.Println("Pushing image...")
	if err := cmd.pushImage(); err != nil {
		return err
	}
	fmt.Println("-->", "done")

	fmt.Println("Releasing...")
	if err := cmd.deployImage(); err != nil {
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

func (cmd *pushCommand) locateImageID() error {
	img, err := cmd.dockerClient.FindImage(cmd.imageRef)
	if err != nil {
		return err
	}

	if img == nil {
		return fmt.Errorf("Could not find a local image tagged %s", cmd.imageRef)
	}

	cmd.imageID = img.ID

	return nil
}

func (cmd *pushCommand) createDeploymentTag() error {
	return cmd.dockerClient.TagImage(cmd.imageID, cmd.imageTag)
}

func (cmd *pushCommand) pushImage() error {
	op := cmd.dockerClient.PushImage(cmd.imageTag)

	var layers = 0
	var doneLayers = 0

	uiprogress.Start()

	bar := uiprogress.AddBar(100)
	bar.AppendCompleted()
	bar.Head = '>'
	bar.PrependFunc(func(b *uiprogress.Bar) string {
		return fmt.Sprintf("--> layer %d of %d", doneLayers, layers)
	})

	for msg := range op.Stream() {
		layers = msg.LayersTotal
		doneLayers = msg.LayersComplete
		bar.Set(msg.Progress)
	}

	uiprogress.Stop()

	return op.Error()
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
