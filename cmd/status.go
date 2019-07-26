package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/machinebox/graphql"
	"github.com/spf13/cobra"
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
		// make a request
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

		// set any variables
		req.Var("appId", appID)

		// set header fields
		// req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", flyToken))

		// define a Context for the request
		ctx := context.Background()

		// run it and capture the response
		var respData interface{}
		if err := client.Run(ctx, req, &respData); err != nil {
			log.Fatal(err)
		}

		log.Println(respData)
	},
}
