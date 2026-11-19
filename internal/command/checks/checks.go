package checks

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

const (
	checksLong = `View the health checks running on an app's Machines. Run ` + "`fly checks list`" + `
to see each check's latest status and output.`
	checksListLong = `List the health checks on each of the app's active Machines, with each
check's status, Machine ID, last update time and latest output. The app
comes from --app or fly.toml.

--check-name limits the table to checks with that name; it doesn't filter
--json output, which maps each Machine ID to its checks.`
)

func New() *cobra.Command {
	commonFlags := flag.Set{flag.App(), flag.AppConfig()}

	cmd := command.New("checks", "Manage health checks", checksLong, nil)
	flag.Add(cmd, commonFlags)

	// fly checks list
	listCmd := command.New("list", "List health checks", checksListLong, runAppCheckList, command.RequireSession, command.RequireAppName)
	listCmd.Aliases = []string{"ls"}
	flag.Add(listCmd, commonFlags,
		flag.String{Name: "check-name", Description: "Filter checks by name"},
	)
	flag.Add(listCmd, flag.JSONOutput())
	cmd.AddCommand(listCmd)

	return cmd
}
