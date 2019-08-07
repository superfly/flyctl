package cmd

import (
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
)

func newAppDeploymentsListCommand() *Command {
	return BuildCommand(runAppDeploymentsList, "deployments", "list app deployments", os.Stdout, true, requireAppName)
}

func runAppDeploymentsList(ctx *CmdContext) error {
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

	req := ctx.FlyClient.NewRequest(query)

	req.Var("appName", ctx.AppName())

	data, err := ctx.FlyClient.Run(req)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"#", "Status", "Reason", "Description", "User", "Created"})

	for _, deployment := range data.App.Deployments.Nodes {
		table.Append([]string{
			strconv.Itoa(deployment.Number),
			deployment.Status,
			deployment.Reason,
			deployment.Description,
			deployment.User.Email,
			deployment.CreatedAt,
		})
	}

	table.Render()

	return nil
}
