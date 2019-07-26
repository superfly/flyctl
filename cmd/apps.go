package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/cli/api"

	"github.com/machinebox/graphql"
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

		if FlyToken == "" {
			fmt.Println("Api token not found")
			os.Exit(1)
			return
		}

		client := graphql.NewClient("https://fly.io/api/v2/graphql")

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

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", FlyToken))

		ctx := context.Background()

		var data api.Query
		if err := client.Run(ctx, req, &data); err != nil {
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
