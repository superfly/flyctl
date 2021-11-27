package orgs

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newInvite() *cobra.Command {
	const (
		long = `Invite a user, by email, to join organization. The invitation will be
sent, and the user will be pending until they respond. See also orgs revoke.
`
		short = "Invite user (by email) to organization"
		usage = "invite [org] [email]"
	)

	cmd := command.New(usage, short, long, runInvite,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runInvite(ctx context.Context) error {
	slug, err := fetchSlug(ctx)
	if err != nil {
		return nil
	}

	email, err := fetchEmail(ctx)
	if err != nil {
		return nil
	}

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
