package orgs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
)

func newRemove() *cobra.Command {
	const (
		long = `Remove a user from an organization. User must have accepted a previous
invitation to join (if not, see orgs revoke).
`
		short = "Remove a user from an organization"
		usage = "remove [slug] [email]"
	)

	cmd := command.New(usage, short, long, runRemove,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runRemove(ctx context.Context) error {
	slug, err := slugFromFirstArgOrSelect(ctx)
	if err != nil {
		return nil
	}

	email, err := emailFromSecondArgOrPrompt(ctx)
	if err != nil {
		return nil
	}

	org, err := detailsFromSlug(ctx, slug)
	if err != nil {
		return err
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

	client := client.FromContext(ctx).API()
	if _, _, err := client.DeleteOrganizationMembership(ctx, org.ID, id); err != nil {
		return fmt.Errorf("failed removing user %s from %s: %w", email, org.Name, err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "successfuly removed user %s from %s\n", email, org.Name)

	return nil
}
