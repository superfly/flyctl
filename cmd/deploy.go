package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func newAppDeployCommand() *cobra.Command {
	deploy := &appDeployCommand{}

	cmd := &cobra.Command{
		Use:   "deploy [flags] <image>",
		Short: "deploy images to an app",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return deploy.Init(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploy.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&deploy.appName, "app", "a", "", `the app name to use`)

	return cmd
}

type appDeployCommand struct {
	client  *api.Client
	appName string
	image   string
}

func (cmd *appDeployCommand) Init(args []string) error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	cmd.client = client

	if cmd.appName == "" {
		cmd.appName = flyctl.CurrentAppName()
	}
	if cmd.appName == "" {
		return fmt.Errorf("no app specified")
	}

	cmd.image = args[0]

	return nil
}

func (cmd *appDeployCommand) Run(args []string) error {
	query := `
			mutation($input: DeployImageInput!) {
				deployImage(input: $input) {
					deployment {
						id
						app {
							runtime
							status
							appUrl
						}
						status
						currentPhase
						release {
							version
						}
					}
				}
			}
		`

	req := cmd.client.NewRequest(query)

	req.Var("input", map[string]string{
		"appId": cmd.appName,
		"image": cmd.image,
	})

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	log.Println(data)

	return nil
}
