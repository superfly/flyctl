package cmd

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
)

func newDomainsCommand(client *client.Client) *Command {
	domainsStrings := docstrings.Get("domains")
	cmd := BuildCommandKS(nil, nil, domainsStrings, client, requireSession)
	cmd.Hidden = true

	listStrings := docstrings.Get("domains.list")
	listCmd := BuildCommandKS(cmd, runDomainsList, listStrings, client, requireSession)
	listCmd.Args = cobra.MaximumNArgs(1)

	showCmd := BuildCommandKS(cmd, runDomainsShow, docstrings.Get("domains.show"), client, requireSession)
	showCmd.Args = cobra.ExactArgs(1)

	addCmd := BuildCommandKS(cmd, runDomainsCreate, docstrings.Get("domains.add"), client, requireSession)
	addCmd.Args = cobra.MaximumNArgs(2)

	registerCmd := BuildCommandKS(cmd, runDomainsRegister, docstrings.Get("domains.register"), client, requireSession)
	registerCmd.Args = cobra.MaximumNArgs(2)
	registerCmd.Hidden = true

	return cmd
}

func runDomainsList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	var orgSlug string
	if len(cmdCtx.Args) == 0 {
		org, err := selectOrganization(ctx, cmdCtx.Client.API(), "")
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	} else {
		// TODO: Validity check on org
		orgSlug = cmdCtx.Args[0]
	}

	domains, err := cmdCtx.Client.API().GetDomains(ctx, orgSlug)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(domains)
		return nil
	}

	table := tablewriter.NewWriter(cmdCtx.Out)

	table.SetHeader([]string{"Domain", "Registration Status", "DNS Status", "Created"})

	for _, domain := range domains {
		table.Append([]string{domain.Name, *domain.RegistrationStatus, *domain.DnsStatus, presenters.FormatRelativeTime(domain.CreatedAt)})
	}

	table.Render()

	return nil
}

func runDomainsShow(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	name := cmdCtx.Args[0]

	domain, err := cmdCtx.Client.API().GetDomain(ctx, name)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(domain)
		return nil
	}

	cmdCtx.Statusf("domains", cmdctx.STITLE, "Domain\n")
	fmtstring := "%-20s: %-20s\n"
	cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Name", domain.Name)
	cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Organization", domain.Organization.Slug)
	cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Registration Status", *domain.RegistrationStatus)
	if *domain.RegistrationStatus == "REGISTERED" {
		cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Expires At", presenters.FormatTime(domain.ExpiresAt))

		autorenew := ""
		if *domain.AutoRenew {
			autorenew = "Enabled"
		} else {
			autorenew = "Disabled"
		}

		cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Auto Renew", autorenew)
	}

	cmdCtx.StatusLn()
	cmdCtx.Statusf("domains", cmdctx.STITLE, "DNS\n")
	cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Status", *domain.DnsStatus)
	if *domain.RegistrationStatus == "UNMANAGED" {
		cmdCtx.Statusf("domains", cmdctx.SINFO, fmtstring, "Nameservers", strings.Join(*domain.ZoneNameservers, " "))
	}

	return nil
}

func runDomainsCreate(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	var org *api.Organization
	var name string
	var err error

	if len(cmdCtx.Args) == 0 {
		org, err = selectOrganization(ctx, cmdCtx.Client.API(), "")
		if err != nil {
			return err
		}

		prompt := &survey.Input{Message: "Domain name to add"}
		err := survey.AskOne(prompt, &name)
		checkErr(err)

		// TODO: Add some domain validation here
	} else if len(cmdCtx.Args) == 2 {
		org, err = cmdCtx.Client.API().GetOrganizationBySlug(ctx, cmdCtx.Args[0])
		if err != nil {
			return err
		}
		name = cmdCtx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	fmt.Printf("Creating domain %s in organization %s\n", name, org.Slug)

	domain, err := cmdCtx.Client.API().CreateDomain(org.ID, name)
	if err != nil {
		return err
	}

	fmt.Println("Created domain", domain.Name)

	return nil
}

func runDomainsRegister(_ *cmdctx.CmdContext) error {

	return fmt.Errorf("This command is no longer supported.\n")
}
