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

	// fly checks handlers
	handlersCmd := command.New("handlers", "Manage health check handlers", "", nil, command.RequireSession, command.RequireAppName)
	cmd.AddCommand(handlersCmd)

	// fly checks handlers list
	hListCmd := command.New("list <organization>", "List health check handlers", "", runListChecksHandlers)
	hListCmd.Args = cobra.ExactArgs(1)
	handlersCmd.AddCommand(hListCmd)

	// fly checks handlers create
	hCreateCmd := command.New("create", "Create a health check handler", "", runCreateChecksHandler)
	flag.Add(hCreateCmd,
		flag.String{Name: "type", Description: "The type of handler to create, can be slack or pagerduty"},
		flag.String{Name: "organization", Description: "The organization to add the handler to"},
		flag.String{Name: "name", Description: "The name of the handler"},
		flag.String{Name: "webhook-url", Description: "The Slack webhook url"},
		flag.String{Name: "slack-channel", Description: "The Slack channel"},
		flag.String{Name: "pagerduty-token", Description: "The PagerDuty token"},
	)
	handlersCmd.AddCommand(hCreateCmd)

	// fly checks handlers delete
	hDeleteCmd := command.New("delete <organization> <handler-name>", "Delete a health check handler", "", runDeleteChecksHandler)
	hDeleteCmd.Args = cobra.ExactArgs(2)
	handlersCmd.AddCommand(hDeleteCmd)

	return cmd
}
