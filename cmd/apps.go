package cmd

import (
	"os"

	"github.com/olekukonko/tablewriter"
)

func newAppListCommand() *Command {
	return BuildCommand(runAppsList, "apps", "list apps", os.Stdout, true)
}

func runAppsList(ctx *CmdContext) error {
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

	req := ctx.FlyClient.NewRequest(query)

	data, err := ctx.FlyClient.Run(req)
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
