package cmd

import (
	"log"
	"os"

	"github.com/machinebox/graphql"
	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(appsCmd)
}

var appsCmd = &cobra.Command{
	Use: "apps",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Run: func(cmd *cobra.Command, args []string) {
		client := api.NewClient("https://fly.io", flyToken)

		req := graphql.NewRequest(`
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
		`)

		var resp api.Query
		apps, err := client.Run(req, &resp)
		if err != nil {
			log.Fatal(err)
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Name", "Owner", "Runtime"})

		for _, app := range data.Apps.Nodes {
			table.Append([]string{app.Name, app.Organization.Slug, app.Runtime})
		}

		table.Render()
	},
}
