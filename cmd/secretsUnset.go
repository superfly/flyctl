package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func newAppSecretsUnsetCommand() *cobra.Command {
	unset := &appSecretsUnsetCommand{}

	cmd := &cobra.Command{
		Use:   "unset [flags] NAME NAME ...",
		Short: "remove encrypted secrets",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return unset.Init(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return unset.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&unset.appName, "app", "a", "", `the app name to use`)

	return cmd
}

type appSecretsUnsetCommand struct {
	client  *api.Client
	appName string
}

func (cmd *appSecretsUnsetCommand) Init(args []string) error {
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

	return nil
}

func (cmd *appSecretsUnsetCommand) Run(args []string) error {
	input := api.UnsetSecretsInput{AppID: cmd.appName, Keys: args}

	query := `
			mutation ($input: UnsetSecretsInput!) {
				unsetSecrets(input: $input) {
					deployment {
						id
						status
					}
				}
			}
		`

	req := cmd.client.NewRequest(query)
	req.Var("input", input)

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", data)

	return nil
}
