package orgs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

func newRemove() *cobra.Command {
	const (
		long = `Remove a user from an organization. User must have accepted a previous
invitation to join (if not, see orgs revoke).
`
		short = "Remove a user from an organization"
		usage = "remove [org] [email]"
	)

	cmd := command.New(usage, short, long, runRemove,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runRemove(ctx context.Context) error {
	slug, err := fetchSlug(ctx)
	if err != nil {
		return err
	}

	email, err := fetchEmail(ctx)
	if err != nil {
		return err
	}

	org, err := retrieveOrgBySlug(ctx, slug)
	if err != nil {
		return err
	}

	var userId string

	// TODO: no bueno, aka: BAD mojo
	// iterate ovver org.Members.Edges and check wether userEmail is in there otherwise return not found error
	for _, m := range org.Members.Edges {
		if m.Node.Email == email {
			userId = m.Node.ID

			break
		}
	}
	if userId == "" {
		return errors.New("user not found")
	}

	client := client.FromContext(ctx).API()

	if _, _, err = client.DeleteOrganizationMembership(ctx, org.ID, userId); err != nil {
		return fmt.Errorf("failed removing member %s: %w", email, err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "removed user %s\n", email)

	return nil
}
