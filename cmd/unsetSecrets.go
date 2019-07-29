package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/machinebox/graphql"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
)

// var appName string

func init() {
	secretsCmd.AddCommand(unsetSecretsCmd)

	unsetSecretsCmd.PersistentFlags().StringVarP(&appName, "app_name", "a", "", "fly app name")
}

var unsetSecretsCmd = &cobra.Command{
	Use: "unset",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		input := api.UnsetSecretsInput{AppID: appName, Keys: args}

		// fmt.Println(input)
		// panic(input)

		if FlyToken == "" {
			fmt.Println("Api token not found")
			os.Exit(1)
			return
		}

		client := graphql.NewClient("https://fly.io/api/v2/graphql")

		req := graphql.NewRequest(`
		    mutation ($input: UnsetSecretsInput!) {
					unsetSecrets(input: $input) {
						deployment {
							id
							status
						}
					}
				}
		`)

		req.Var("input", input)

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", FlyToken))

		ctx := context.Background()

		var data api.Query
		if err := client.Run(ctx, req, &data); err != nil {
			log.Fatal(err)
		}

		log.Printf("%+v\n", data)
	},
}
