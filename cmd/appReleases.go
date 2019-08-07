package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppReleasesListCommand() *Command {
	return BuildCommand(runAppReleasesList, "releases", "list app releases", os.Stdout, true, requireAppName)
}

func runAppReleasesList(ctx *CmdContext) error {
	releases, err := ctx.FlyClient.GetAppReleases(ctx.AppName(), 25)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.ReleasePresenter{Releases: releases})

	// query := `
	// 		query ($appName: String!) {
	// 			app(id: $appName) {
	// 				deployments {
	// 					nodes {
	// 						id
	// 						number
	// 						status
	// 						inProgress
	// 						currentPhase
	// 						reason
	// 						description
	// 						user {
	// 							email
	// 						}
	// 						createdAt
	// 						updatedAt
	// 					}
	// 				}
	// 			}
	// 		}
	// 	`

	// req := ctx.FlyClient.NewRequest(query)

	// req.Var("appName", ctx.AppName())
	// req.Var("limit", 25)

	// data, err := ctx.FlyClient.Run(req)
	// if err != nil {
	// 	return err
	// }

	// table := tablewriter.NewWriter(os.Stdout)
	// table.SetHeader([]string{"#", "Status", "Reason", "Description", "User", "Created"})

	// for _, deployment := range data.App.Deployments.Nodes {
	// 	table.Append([]string{
	// 		strconv.Itoa(deployment.Number),
	// 		deployment.Status,
	// 		deployment.Reason,
	// 		deployment.Description,
	// 		deployment.User.Email,
	// 		deployment.CreatedAt,
	// 	})
	// }

	// table.Render()

	// return nil
}
