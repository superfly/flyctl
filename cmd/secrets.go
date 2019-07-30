package cmd

import (
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/manifest"

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
	RunE: func(cmd *cobra.Command, args []string) error {

		if appName == "" {
			manifest, err := manifest.LoadManifest("fly.toml")
			if err != nil {
				panic(err)
			}
			appName = manifest.AppID
		}

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		req := client.NewRequest(`
			query ($appName: String!) {
				app(id: $appName) {
					secrets
				}
			}
		`)

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
