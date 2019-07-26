package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

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
		// make a request
		req := graphql.NewRequest(`
    query {
        apps {
					nodes {
						id
						name
						runtime
					}
        }
    }
`)

		// set any variables
		// req.Var("key", "value")

		// set header fields
		// req.Header.Set("Cache-Control", "no-cache")

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", FlyToken))

		// define a Context for the request
		ctx := context.Background()

		// run it and capture the response
		var respData api.Apps
		if err := client.Run(ctx, req, &respData); err != nil {
			log.Fatal(err)
		}

		log.Println(respData)

		for _, app := range respData.Apps.Nodes {
			fmt.Println(app)
		}
	},
}
