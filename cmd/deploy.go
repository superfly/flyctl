package cmd

import (
	"errors"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func init() {
	rootCmd.AddCommand(deployCmd)
	addAppFlag(deployCmd)
}

var deployCmd = &cobra.Command{
	Use:   "deploy [flags] <image>",
	Short: "deploy images to an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := viper.GetString(flyctl.ConfigAppName)
		if appName == "" {
			return errors.New("No app provided")
		}

		image := args[0]

		client, err := api.NewClient()
		if err != nil {
			return err
		}

		query := `
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
		`

		req := client.NewRequest(query)

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
