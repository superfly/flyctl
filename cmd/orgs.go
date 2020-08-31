package cmd

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
)

func newOrgsCommand() *Command {
	orgsStrings := docstrings.Get("orgs")
	orgscmd := BuildCommandKS(nil, nil, orgsStrings, os.Stdout, requireSession)

	orgsListStrings := docstrings.Get("orgs.list")
	BuildCommandKS(orgscmd, runOrgsList, orgsListStrings, os.Stdout, requireSession)

	orgsShowStrings := docstrings.Get("orgs.show")
	orgsShowCommand := BuildCommandKS(orgscmd, runOrgsShow, orgsShowStrings, os.Stdout, requireSession)
	orgsShowCommand.Args = cobra.ExactArgs(1)

	orgsInviteStrings := docstrings.Get("orgs.invite")
	orgsInviteCommand := BuildCommandKS(orgscmd, runOrgsInvite, orgsInviteStrings, os.Stdout, requireSession)
	orgsInviteCommand.Args = cobra.MaximumNArgs(2)

	orgsRevokeStrings := docstrings.Get("orgs.revoke")
	orgsRevokeCommand := BuildCommandKS(orgscmd, runOrgsRevoke, orgsRevokeStrings, os.Stdout, requireSession)
	orgsRevokeCommand.Args = cobra.MaximumNArgs(2)

	orgsRemoveStrings := docstrings.Get("orgs.remove")
	orgsRemoveCommand := BuildCommandKS(orgscmd, runOrgsRemove, orgsRemoveStrings, os.Stdout, requireSession)
	orgsRemoveCommand.Args = cobra.MaximumNArgs(2)

	orgsCreateStrings := docstrings.Get("orgs.create")
	orgsCreateCommand := BuildCommandKS(orgscmd, runOrgsCreate, orgsCreateStrings, os.Stdout, requireSession)
	orgsCreateCommand.Args = cobra.MaximumNArgs(1)

	orgsDeleteStrings := docstrings.Get("orgs.delete")
	orgsDeleteCommand := BuildCommandKS(orgscmd, runOrgsDelete, orgsDeleteStrings, os.Stdout, requireSession)
	orgsDeleteCommand.Args = cobra.MaximumNArgs(1)

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

	printOrg(personalOrganization, true)

	for _, o := range organizations {
		if o.ID != personalOrganization.ID {
			printOrg(o, false)
		}
	}

	return nil
}

func printOrg(o api.Organization, headers bool) {

	if headers {
		fmt.Printf("%-20s %-20s %-10s\n", "Name", "Slug", "Type")
		fmt.Printf("%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Printf("%-20s %-20s %-10s\n", o.Name, o.Slug, o.Type)

}

func runOrgsShow(ctx *cmdctx.CmdContext) error {
	asJSON := ctx.OutputJSON()
	orgslug := ctx.Args[0]

	org, err := ctx.Client.API().GetOrganizationBySlug(orgslug)

	if err != nil {
		return err
	}

	if asJSON {
		ctx.WriteJSON(org)
		return nil
	}

	ctx.Statusf("fyctl", cmdctx.STITLE, "Organization\n")

	ctx.Statusf("flyctl", cmdctx.SINFO, "%-10s: %-20s\n", "Name", org.Name)
	ctx.Statusf("flyctl", cmdctx.SINFO, "%-10s: %-20s\n", "Slug", org.Slug)
	ctx.Statusf("flyctl", cmdctx.SINFO, "%-10s: %-20s\n", "Type", org.Type)

	ctx.StatusLn()

	ctx.Statusf("fyctl", cmdctx.STITLE, "Summary\n")

	ctx.Statusf("flyctl", cmdctx.SINFO, "You have %s permissions on this organizaton\n", org.ViewerRole)
	ctx.Statusf("flyctl", cmdctx.SINFO, "There are %d DNS zones associated with this organization\n", len(org.DNSZones.Nodes))
	ctx.Statusf("flyctl", cmdctx.SINFO, "There are %d members associated with this organization\n", len(org.Members.Edges))

	ctx.StatusLn()

	ctx.Statusf("fyctl", cmdctx.STITLE, "Organization Members\n")

	membertable := tablewriter.NewWriter(ctx.Out)
	membertable.SetHeader([]string{"Name", "Email", "Role"})

	for _, m := range org.Members.Edges {
		membertable.Append([]string{m.Node.Name, m.Node.Email, m.Role})
	}
	membertable.Render()

	return nil
}

func runOrgsInvite(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Invite Not implemented")
}

func runOrgsCreate(ctx *cmdctx.CmdContext) error {
	asJSON := ctx.OutputJSON()

	orgname := ctx.Args[0]

	organization, err := ctx.Client.API().CreateOrganization(orgname)
	if err != nil {
		return err
	}

	if asJSON {
		ctx.WriteJSON(organization)
	} else {
		printOrg(*organization, true)
	}

	return nil
}

func runOrgsRemove(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Remove Not implemented")
}

func runOrgsRevoke(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Revoke Not implemented")
}

func runOrgsDelete(ctx *cmdctx.CmdContext) error {
	orgslug := ctx.Args[0]

	org, err := ctx.Client.API().GetOrganizationBySlug(orgslug)

	if err != nil {
		return err
	}

	confirmed := confirm(fmt.Sprintf("Are you sure you want to delete the %s organization?", orgslug))

	if !confirmed {
		return nil
	}

	_, err = ctx.Client.API().DeleteOrganization(org.ID)

	if err != nil {
		return err
	}

	return nil
}
