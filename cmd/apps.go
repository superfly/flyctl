package cmd

import (
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
)

func newAppListCommand() *cobra.Command {
	list := &appListCommand{}

	cmd := &cobra.Command{
		Use:   "apps",
		Short: "list apps",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return list.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return list.Run(args)
		},
	}

	return cmd
}

type appListCommand struct {
	client *api.Client
}

func (cmd *appListCommand) Init() error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	cmd.client = client

	return nil
}

func (cmd *appListCommand) Run(args []string) error {
	query := `
		query {
			apps {
				nodes {
					id
					name
					organization {
						slug
					}
					runtime
				}
			}
		}
		`

	req := cmd.client.NewRequest(query)

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Owner", "Runtime"})

	for _, app := range data.Apps.Nodes {
		table.Append([]string{app.Name, app.Organization.Slug, app.Runtime})
	}

	table.Render()

	return nil
}
