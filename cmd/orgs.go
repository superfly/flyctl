package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"
)

func newOrgsCommand() *Command {
	orgsStrings := docstrings.KeyStrings{Usage: "orgs",
		Short: "Commands for managing Fly organizations",
		Long: `Commands for managing Fly organizations. list, create, show and 
	destroy organizations. 
	Organization admins can also invite or remove users from Organizations. 
	`}

	orgscmd := BuildCommandKS(nil, nil, orgsStrings, os.Stdout, requireSession)

	orgsListStrings := docstrings.KeyStrings{Usage: "list",
		Short: "Lists organizations for current user",
		Long: `Lists organizations available to current user.
	`}

	BuildCommandKS(orgscmd, runOrgsList, orgsListStrings, os.Stdout, requireSession)

	orgsShowStrings := docstrings.KeyStrings{Usage: "show <org>", Short: "Show organization", Long: ""}
	orgsShowCommand := BuildCommandKS(orgscmd, runOrgsShow, orgsShowStrings, os.Stdout, requireSession)
	orgsShowCommand.Args = cobra.ExactArgs(1)

	orgsInviteStrings := docstrings.KeyStrings{Usage: "invite <org> <email>", Short: "invite to organization", Long: ""}
	orgsInviteCommand := BuildCommandKS(orgscmd, runOrgsInvite, orgsInviteStrings, os.Stdout, requireSession)
	orgsInviteCommand.Args = cobra.MaximumNArgs(2)

	orgsRevokeStrings := docstrings.KeyStrings{Usage: "revoke <org> <email>", Short: "revoke an invitation to an organization", Long: ""}
	orgsRevokeCommand := BuildCommandKS(orgscmd, runOrgsRevoke, orgsRevokeStrings, os.Stdout, requireSession)
	orgsRevokeCommand.Args = cobra.MaximumNArgs(2)

	orgsRemoveStrings := docstrings.KeyStrings{Usage: "remove <org> <email>", Short: "revoke an invitation to an organization", Long: ""}
	orgsRemoveCommand := BuildCommandKS(orgscmd, runOrgsRemove, orgsRemoveStrings, os.Stdout, requireSession)
	orgsRemoveCommand.Args = cobra.MaximumNArgs(2)

	orgsCreateStrings := docstrings.KeyStrings{Usage: "create <org> <email>", Short: "Create an Organization", Long: ""}
	orgsCreateCommand := BuildCommandKS(orgscmd, runOrgsCreate, orgsCreateStrings, os.Stdout, requireSession)
	orgsCreateCommand.Args = cobra.MaximumNArgs(1)

	orgsDestroyStrings := docstrings.KeyStrings{Usage: "destroy <org>", Short: "Destroy an Organization", Long: ""}
	orgsDestroyCommand := BuildCommandKS(orgscmd, runOrgsDestroy, orgsDestroyStrings, os.Stdout, requireSession)
	orgsDestroyCommand.Args = cobra.MaximumNArgs(1)

	return orgscmd
}

func runOrgsList(cmdctx *cmdctx.CmdContext) error {
	asJSON := cmdctx.OutputJSON()

	personalOrganization, organizations, err := cmdctx.Client.API().GetCurrentOrganizations()
	if err != nil {
		return err
	}

	if asJSON {
		type MyOrgs struct {
			PersonalOrganization api.Organization
			Organizations        []api.Organization
		}
		cmdctx.WriteJSON(MyOrgs{PersonalOrganization: personalOrganization, Organizations: organizations})
		return nil
	}

	fmt.Println("Formatted Print Here")

	return nil
}

func runOrgsShow(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Show Not implemented")
}

func runOrgsInvite(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Invite Not implemented")
}

func runOrgsCreate(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Create Not implemented")
}

func runOrgsRemove(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Remove Not implemented")
}

func runOrgsRevoke(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Revoke Not implemented")
}

func runOrgsDestroy(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Destroy Not implemented")
}
