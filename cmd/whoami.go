package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/machinebox/graphql"
	"github.com/spf13/cobra"
	"github.com/superfly/cli/api"
)

func init() {
	rootCmd.AddCommand(whoami)
}

func init() {
	// whoami.Flags().StringVarP(&appID, "app", "a", "", "App id")
}

var whoami = &cobra.Command{
	Use: "whoami",
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
			currentUser {
				email
			}
    }
`)

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", FlyToken))

		ctx := context.Background()

		var data api.Query
		if err := client.Run(ctx, req, &data); err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Current user: %s\n", data.CurrentUser.Email)
	},
}
