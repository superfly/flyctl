package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flyctl"
)

func init() {
	rootCmd.AddCommand(logOut)
}

func init() {
}

var logOut = &cobra.Command{
	Use: "logout",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := flyctl.ClearSavedAccessToken(); err != nil {
			log.Fatalln(err)
		}

		fmt.Println("Session removed")
	},
}
