package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"

	"github.com/spf13/cobra"
)

func newAppDeploymentsListCommand() *cobra.Command {
	list := &appDeploymentsListCommand{}

	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "list app deployments",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return list.Init(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return list.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&list.appName, "app", "a", "", `the app name to use`)

	return cmd
}

type appDeploymentsListCommand struct {
	client  *api.Client
	appName string
}

func (cmd *appDeploymentsListCommand) Init(args []string) error {
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

func (cmd *appDeploymentsListCommand) Run(args []string) error {
	query := `
			query ($appName: String!) {
				app(id: $appName) {
					deployments {
						nodes {
							id
							number
							status
							inProgress
							currentPhase
							reason
							description
							user {
								email
							}
							createdAt
							updatedAt
						}
					}    
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
	table.SetHeader([]string{"#", "Status", "Reason", "Description", "User", "Created", "Modified"})

	for _, deployment := range data.App.Deployments.Nodes {
		table.Append([]string{
			strconv.Itoa(deployment.Number),
			deployment.Status,
			deployment.Reason,
			deployment.Description,
			deployment.User.Email,
			deployment.CreatedAt,
			deployment.UpdatedAt,
		})
	}

	table.Render()

	return nil
}
