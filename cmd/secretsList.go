package cmd

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"

	"github.com/spf13/cobra"
)

func newAppSecretsListCommand() *cobra.Command {
	set := &appSecretsListCommand{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "list app secret names",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return set.Init(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return set.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&set.appName, "app", "a", "", `the app name to use`)

	return cmd
}

type appSecretsListCommand struct {
	client  *api.Client
	appName string
}

func (cmd *appSecretsListCommand) Init(args []string) error {
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

func (cmd *appSecretsListCommand) Run(args []string) error {
	query := `
			query ($appName: String!) {
				app(id: $appName) {
					secrets
				}
			}
		`

	req := cmd.client.NewRequest(query)

	req.Var("appName", cmd.appName)

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name"})

	for _, secret := range data.App.Secrets {
		table.Append([]string{secret})
	}

	table.Render()

	return nil
}
