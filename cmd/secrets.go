package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"

	"github.com/machinebox/graphql"
	"github.com/spf13/cobra"
)

var appName string

func init() {
	rootCmd.AddCommand(secretsCmd)

	secretsCmd.PersistentFlags().StringVarP(&appName, "app_name", "a", "", "fly app name")
}

var secretsCmd = &cobra.Command{
	Use: "secrets",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Run: func(cmd *cobra.Command, args []string) {

		if flyToken == "" {
			fmt.Println("Api token not found")
			os.Exit(1)
			return
		}

		client := graphql.NewClient("https://fly.io/api/v2/graphql")

		req := graphql.NewRequest(`
    query ($appName: String!) {
			app(id: $appName) {
				secrets
			}
		}
`)

		req.Var("appName", appName)

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", flyToken))

		ctx := context.Background()

		var data api.Query
		if err := client.Run(ctx, req, &data); err != nil {
			log.Fatal(err)
		}

		log.Println(data)

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Name"})

		for _, secret := range data.App.Secrets {
			table.Append([]string{secret})
		}

		table.Render()
	},
}
