package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/machinebox/graphql"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/cli/api"
	"github.com/superfly/cli/manifest"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var appID string

func init() {
	statusCmd.Flags().StringVarP(&appID, "app", "a", "", "App id")
}

var statusCmd = &cobra.Command{
	Use: "status",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Run: func(cmd *cobra.Command, args []string) {
		client := graphql.NewClient("https://fly.io/api/v2/graphql")
		req := graphql.NewRequest(`
  query($appId: String!) {
    app(id: $appId) {
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
`)

		if appID == "" {
			manifest, err := manifest.LoadManifest("fly.toml")
			if err != nil {
				panic(err)
			}
			appID = manifest.AppID
		}

		req.Var("appId", appID)

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", FlyToken))

		ctx := context.Background()

		var data api.Query
		if err := client.Run(ctx, req, &data); err != nil {
			log.Fatal(err)
		}

		app := data.App

		renderAppInfo(app)

		if len(app.Services) > 0 {
			fmt.Println()
			renderServicesList(app)
			fmt.Println()
			renderAllocationsList(app)
		}
	},
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
