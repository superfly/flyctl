package cmd


import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flyctl"
)

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
	Use:   "version",
	Short: "show flyctl version information",
		Run: func(cmd *cobra.Command, args []string)  {
			fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
		},
	}

	return cmd
}


