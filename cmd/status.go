package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"
)

func newAppStatusCommand() *Command {
	cmd := BuildCommand(runAppStatus, "status", "show app status", os.Stdout, true, requireAppName)

	return cmd
}

func runAppStatus(ctx *CmdContext) error {
	query := `
			query($appName: String!) {
				app(name: $appName) {
					id
					name
					version
					status
					appUrl
					organization {
						slug
					}
					services {
						id
						name
						status
						allocations {
							id
							name
							status
							region
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

	renderAppInfo(data.App)

	if len(data.App.Services) > 0 {
		fmt.Println()
		renderServicesList(data.App)
		fmt.Println()
		renderAllocationsList(data.App)
	}

	return nil
}

func renderAppInfo(app api.App) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.AppendBulk([][]string{
		[]string{"Name", app.Name},
		[]string{"Owner", app.Organization.Slug},
		[]string{"Version", strconv.Itoa(app.Version)},
		[]string{"Status", app.Status},
	})
	if app.AppURL == "" {
		table.Append([]string{"App URL", "N/A"})
	} else {
		table.Append([]string{"App URL", app.AppURL})
	}

	table.Render()
}

func renderServicesList(app api.App) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, service := range app.Services {
		table.Append([]string{service.Name, service.Status})
	}

	table.Render()
}

func renderAllocationsList(app api.App) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Status", "Region", "Created", "Modified"})

	for _, service := range app.Services {
		for _, alloc := range service.Allications {
			table.Append([]string{alloc.Name, alloc.Status, alloc.Region, alloc.CreatedAt, alloc.UpdatedAt})
		}
	}

	table.Render()
}
