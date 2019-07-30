package cmd

import (
	"errors"
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.AddCommand(secretsCmd)
	addAppFlag(secretsCmd)
}

var secretsCmd = &cobra.Command{
	Use: "secrets",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := viper.GetString(flyctl.ConfigAppName)
		if appName == "" {
			return errors.New("No app provided")
		}

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		query := `
			query ($appName: String!) {
				app(id: $appName) {
					secrets
				}
			}
		`

		req := client.NewRequest(query)

		req.Var("appName", appName)

		data, err := client.Run(req)
		if err != nil {
			return err
		}

		log.Println(data)

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Name"})

		for _, secret := range data.App.Secrets {
			table.Append([]string{secret})
		}

		table.Render()

		return nil
	},
}
