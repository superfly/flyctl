package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/machinebox/graphql"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(deployCmd)
}

var deployCmd = &cobra.Command{
	Use: "deploy",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(args)

		client := graphql.NewClient("https://fly.io/api/v2/graphql")
		// make a request
		req := graphql.NewRequest(`
  mutation($input: DeployImageInput!) {
    deployImage(input: $input) {
      deployment {
        id
        app {
          runtime
          status
          appUrl
        }
        status
        currentPhase
        release {
          version
        }
      }
    }
  }
`)

		req.Var("input", map[string]string{
			"appId": "deno-test-1",
			"image": "registry.hub.docker.com/michaeldwan/something:latest",
		})

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", flyToken))

		ctx := context.Background()

		var respData interface{}
		if err := client.Run(ctx, req, &respData); err != nil {
			log.Fatal(err)
		}

		log.Println(respData)
	},
}
