package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func newAppStatusCommand() *cobra.Command {
	status := &appStatusCommand{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "show app status",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return status.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return status.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&status.appName, "app", "a", "", `app to run command against`)

	return cmd
}

type appStatusCommand struct {
	client  *api.Client
	appName string
}

func (cmd *appStatusCommand) Init() error {
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

func (cmd *appStatusCommand) Run(args []string) error {
	query := `
			query($appName: String!) {
				app(name: $appName) {
					id
					name
					version
					runtime
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

	req := cmd.client.NewRequest(query)

	req.Var("appName", cmd.appName)

	data, err := cmd.client.Run(req)
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
		[]string{"Runtime", app.Runtime},
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
