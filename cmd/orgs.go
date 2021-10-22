package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newOrgsCommand(client *client.Client) *Command {
	orgsStrings := docstrings.Get("orgs")
	orgscmd := BuildCommandKS(nil, nil, orgsStrings, client, requireSession)

	orgsListStrings := docstrings.Get("orgs.list")
	BuildCommandKS(orgscmd, runOrgsList, orgsListStrings, client, requireSession)

	orgsShowStrings := docstrings.Get("orgs.show")
	orgsShowCommand := BuildCommandKS(orgscmd, runOrgsShow, orgsShowStrings, client, requireSession)
	orgsShowCommand.Args = cobra.ExactArgs(1)

	orgsInviteStrings := docstrings.Get("orgs.invite")
	orgsInviteCommand := BuildCommandKS(orgscmd, runOrgsInvite, orgsInviteStrings, client, requireSession)
	orgsInviteCommand.Args = cobra.MaximumNArgs(2)

	orgsRevokeStrings := docstrings.Get("orgs.revoke")
	orgsRevokeCommand := BuildCommandKS(orgscmd, runOrgsRevoke, orgsRevokeStrings, client, requireSession)
	orgsRevokeCommand.Args = cobra.MaximumNArgs(2)

	orgsRemoveStrings := docstrings.Get("orgs.remove")
	orgsRemoveCommand := BuildCommandKS(orgscmd, runOrgsRemove, orgsRemoveStrings, client, requireSession)
	orgsRemoveCommand.Args = cobra.MaximumNArgs(2)

	orgsCreateStrings := docstrings.Get("orgs.create")
	orgsCreateCommand := BuildCommandKS(orgscmd, runOrgsCreate, orgsCreateStrings, client, requireSession)
	orgsCreateCommand.Args = cobra.RangeArgs(0, 1)

	orgsDeleteStrings := docstrings.Get("orgs.delete")
	orgsDeleteCommand := BuildCommandKS(orgscmd, runOrgsDelete, orgsDeleteStrings, client, requireSession)
	orgsDeleteCommand.Args = cobra.ExactArgs(1)

	return orgscmd
}

func runOrgsList(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	asJSON := cmdCtx.OutputJSON()

	personalOrganization, organizations, err := cmdCtx.Client.API().GetCurrentOrganizations(ctx)
	if err != nil {
		return err
	}

	if asJSON {
		type MyOrgs struct {
			PersonalOrganization api.Organization
			Organizations        []api.Organization
		}
		cmdCtx.WriteJSON(MyOrgs{PersonalOrganization: personalOrganization, Organizations: organizations})
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

func printInvite(in api.Invitation, headers bool) {

	if headers {
		fmt.Printf("%-20s %-20s %-10s\n", "Org", "Email", "Redeemed")
		fmt.Printf("%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Printf("%-20s %-20s %-10t\n", in.Organization.Slug, in.Email, in.Redeemed)
}

func runOrgsShow(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	asJSON := cmdCtx.OutputJSON()
	orgslug := cmdCtx.Args[0]

	org, err := cmdCtx.Client.API().GetOrganizationBySlug(ctx, orgslug)

	if err != nil {
		return err
	}

	if asJSON {
		cmdCtx.WriteJSON(org)
		return nil
	}

	cmdCtx.Statusf("orgs", cmdctx.STITLE, "Organization\n")

	cmdCtx.Statusf("orgs", cmdctx.SINFO, "%-10s: %-20s\n", "Name", org.Name)
	cmdCtx.Statusf("orgs", cmdctx.SINFO, "%-10s: %-20s\n", "Slug", org.Slug)
	cmdCtx.Statusf("orgs", cmdctx.SINFO, "%-10s: %-20s\n", "Type", org.Type)

	cmdCtx.StatusLn()

	cmdCtx.Statusf("orgs", cmdctx.STITLE, "Summary\n")

	cmdCtx.Statusf("orgs", cmdctx.SINFO, "You have %s permissions on this organizaton\n", org.ViewerRole)
	// ctx.Statusf("orgs", cmdctx.SINFO, "There are %d DNS zones associated with this organization\n", len(org.DNSZones.Nodes))
	cmdCtx.Statusf("orgs", cmdctx.SINFO, "There are %d members associated with this organization\n", len(org.Members.Edges))

	cmdCtx.StatusLn()

	cmdCtx.Statusf("fyctl", cmdctx.STITLE, "Organization Members\n")

	membertable := tablewriter.NewWriter(cmdCtx.Out)
	membertable.SetHeader([]string{"Name", "Email", "Role"})

	for _, m := range org.Members.Edges {
		membertable.Append([]string{m.Node.Name, m.Node.Email, m.Role})
	}
	membertable.Render()

	return nil
}

func runOrgsInvite(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	var orgSlug, userEmail string

	orgType := api.OrganizationTypeShared

	if len(cmdCtx.Args) == 0 {

		org, err := selectOrganization(ctx, cmdCtx.Client.API(), "", &orgType)
		if err != nil {
			return err
		}
		orgSlug = org.Slug

		userEmail, err = inputUserEmail()
		if err != nil {
			return err
		}
	} else if len(cmdCtx.Args) == 2 {
		orgSlug = cmdCtx.Args[0]

		userEmail = cmdCtx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	org, err := cmdCtx.Client.API().GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	out, err := cmdCtx.Client.API().CreateOrganizationInvite(ctx, org.ID, userEmail)
	if err != nil {
		return err
	}

	printInvite(*out, true)

	return nil
}

func runOrgsCreate(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	asJSON := cmdCtx.OutputJSON()

	orgname := ""

	if len(cmdCtx.Args) == 0 {
		prompt := &survey.Input{
			Message: "Enter Organization Name:",
		}
		if err := survey.AskOne(prompt, &orgname); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	} else {
		orgname = cmdCtx.Args[0]
	}

	organization, err := cmdCtx.Client.API().CreateOrganization(ctx, orgname)
	if err != nil {
		return err
	}

	if asJSON {
		cmdCtx.WriteJSON(organization)
	} else {
		printOrg(*organization, true)
	}

	return nil
}

func runOrgsRemove(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	var orgSlug, userEmail string

	orgType := api.OrganizationTypeShared

	if len(cmdCtx.Args) == 0 {

		org, err := selectOrganization(ctx, cmdCtx.Client.API(), "", &orgType)
		if err != nil {
			return err
		}
		orgSlug = org.Slug

		userEmail, err = inputUserEmail()
		if err != nil {
			return err
		}
	} else if len(cmdCtx.Args) == 2 {
		orgSlug = cmdCtx.Args[0]

		userEmail = cmdCtx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	org, err := cmdCtx.Client.API().GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	var userId string

	// iterate ovver org.Members.Edges and check wether userEmail is in there otherwise return not found error
	for _, m := range org.Members.Edges {
		if m.Node.Email == userEmail {
			userId = m.Node.ID
			break
		}
	}
	if userId == "" {
		return errors.New("user not found")
	}

	_, userEmail, err = cmdCtx.Client.API().DeleteOrganizationMembership(ctx, org.ID, userId)
	if err != nil {
		return err
	}

	fmt.Printf("Successfuly removed %s\n", userEmail)

	return nil
}

func runOrgsRevoke(ctx *cmdctx.CmdContext) error {
	return fmt.Errorf("Revoke Not implemented")
}

func runOrgsDelete(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	orgslug := cmdCtx.Args[0]

	org, err := cmdCtx.Client.API().GetOrganizationBySlug(ctx, orgslug)

	if err != nil {
		return err
	}

	confirmed := confirm(fmt.Sprintf("Are you sure you want to delete the %s organization?", orgslug))

	if !confirmed {
		return nil
	}

	_, err = cmdCtx.Client.API().DeleteOrganization(ctx, org.ID)

	if err != nil {
		return err
	}

	return nil
}
