package orgs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newRemove() *cobra.Command {
	const (
		long = `Remove a member from an organization, identified by email. The user must
already be a member; if no member has that email, the command fails.

Pass the organization slug and the email address as arguments. With no
slug, you pick from the organizations you administer in a terminal, and
with no email you're prompted for one; without a terminal, both are
required. There's no confirmation step. Afterwards, the command prints a
link to the organization's offboarding checklist.`
		short = "Remove a user from an organization"
		usage = "remove [slug] [email]"
	)

	cmd := command.New(usage, short, long, runRemove,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runRemove(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)
	selectedOrg, err := OrgFromEnvVarOrFirstArgOrSelect(ctx, AdminOnly)
	if err != nil {
		return nil
	}

	org, err := client.GetDetailedOrganizationBySlug(ctx, selectedOrg.Slug)
	if err != nil {
		return nil
	}

	email, err := emailFromSecondArgOrPrompt(ctx)
	if err != nil {
		return nil
	}

	var id string
	for _, m := range org.Members.Edges {
		if m.Node.Email == email {
			id = m.Node.ID

			break
		}
	}
	if id == "" {
		return errors.New("user not found")
	}

	if _, _, err := client.DeleteOrganizationMembership(ctx, org.ID, id); err != nil {
		return fmt.Errorf("failed removing user %s from %s: %w", email, org.Name, err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "successfully removed user %s from %s\n", email, org.Name)
	fmt.Fprintf(io.Out, "Offboarding Checklist: https://fly.io/dashboard/%s/offboarding", org.Slug)

	return nil
}
