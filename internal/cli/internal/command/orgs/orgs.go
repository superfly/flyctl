package orgs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `Commands for managing Fly organizations. list, create, show and 
destroy organizations. 
Organization admins can also invite or remove users from Organizations.
`
		short = "Commands for managing Fly organizations"
	)

	// TODO: list should also accept the --org param

	orgs := command.New("orgs", short, long, nil)

	orgs.AddCommand(
		newList(),
		newShow(),
		newInvite(),
		newRevoke(),
		newRemove(),
		newCreate(),
		newDelete(),
	)

	return orgs
}

func fetchSlug(ctx context.Context) (name string, err error) {
	if name = flag.FirstArg(ctx); name != "" {
		return
	}

	const msg = "Enter Organization Name:"

	if err = prompt.String(ctx, &name, msg, "", true); prompt.IsNonInteractive(err) {
		err = errors.New("org argument must be specified when not running interactively")
	}

	return
}

func fetchEmail(ctx context.Context) (email string, err error) {
	args := flag.Args(ctx)
	if len(args) > 1 {
		email = args[1]

		return
	}

	const msg = "User email:"

	if err = prompt.String(ctx, &email, msg, "", true); prompt.IsNonInteractive(err) {
		err = errors.New("email argument must be specified when not running interactively")
	}

	return
}

func retrieveOrgBySlug(ctx context.Context, slug string) (org *api.OrganizationDetails, err error) {
	client := client.FromContext(ctx).API()

	if org, err = client.GetOrganizationBySlug(ctx, slug); err != nil {
		err = errors.Wrapf(err, "failed retrieving organization %s details", slug)
	}

	return
}

func printInvite(in api.Invitation, headers bool) {

	if headers {
		fmt.Printf("%-20s %-20s %-10s\n", "Org", "Email", "Redeemed")
		fmt.Printf("%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Printf("%-20s %-20s %-10t\n", in.Organization.Slug, in.Email, in.Redeemed)
}

func runOrgsInvite(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

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

func runOrgsRemove(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

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

func runOrgsRevoke(_ *cmdctx.CmdContext) error {
	return fmt.Errorf("Revoke Not implemented")
}

func runOrgsDelete(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

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
