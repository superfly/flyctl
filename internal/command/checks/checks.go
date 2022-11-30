package checks

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func New() *cobra.Command {
	commonFlags := flag.Set{flag.App(), flag.AppConfig()}

	cmd := command.New("checks", "Manage health checks", "", nil)
	flag.Add(cmd, commonFlags)

	// fly checks list
	listCmd := command.New("list", "List health checks", "", runAppCheckList, command.RequireSession, command.RequireAppName)
	flag.Add(listCmd, commonFlags,
		flag.String{Name: "check-name", Description: "Filter checks by name"},
	)
	cmd.AddCommand(listCmd)
	return cmd
}
