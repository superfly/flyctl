package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/manifest"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var appID string

func init() {
	statusCmd.Flags().StringP("app", "a", "", "App name")
	viper.BindPFlags(statusCmd.Flags())
	// viper.
	// statusCmd.Flags().StringVarP(&appID, "app", "a", "", "App id")
}

var statusCmd = &cobra.Command{
	Use: "status",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// panic(viper.GetString("api_base_url"))

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		req := client.NewRequest(`
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

		data, err := client.Run(req)
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
