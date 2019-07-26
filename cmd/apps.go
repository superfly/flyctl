package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/superfly/cli/api"

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
		client := api.NewClient("https://fly.io", flyToken)

		apps, err := client.Apps(context.Background())
		if err != nil {
			log.Fatal(err)
		}

		for _, app := range apps.Apps.Nodes {
			fmt.Println(app)
		}
	},
}
