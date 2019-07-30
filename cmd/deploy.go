package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/manifest"
)

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.Flags().StringVarP(&appName, "app", "a", "", "App Name")
}

var deployCmd = &cobra.Command{
	Use: "deploy",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		if appName == "" {
			manifest, err := manifest.LoadManifest("fly.toml")
			if err != nil {
				panic(err)
			}
			appName = manifest.AppID
		}

		image := args[0]

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		req := client.NewRequest(`
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
			"appId": appName,
			"image": image,
		})

		data, err := client.Run(req)
		if err != nil {
			return err
		}

		log.Println(data)

		return nil
	},
}
