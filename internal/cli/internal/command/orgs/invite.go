package orgs

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newInvite() *cobra.Command {
	const (
		long = `Invite a user, by email, to join organization. The invitation will be
sent, and the user will be pending until they respond. See also orgs revoke.
`
		short = "Invite user (by email) to organization"
		usage = "invite [slug] [email]"
	)

	cmd := command.New(usage, short, long, runInvite,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runInvite(ctx context.Context) error {
	slug, err := slugFromFirstArgOrSelect(ctx)
	if err != nil {
		return nil
	}

	email, err := emailFromSecondArgOrPrompt(ctx)
	if err != nil {
		return nil
	}

	client := client.FromContext(ctx).API()

	org, err := detailsFromSlug(ctx, slug)
	if err != nil {
		return err
	}

	inv, err := client.CreateOrganizationInvite(ctx, org.ID, email)
	if err != nil {
		return fmt.Errorf("failed inviting %s to %s: %w", email, org.Name, err)
	}

	cfg := config.FromContext(ctx)
	io := iostreams.FromContext(ctx)

	if cfg.JSONOutput {
		_ = render.JSON(io.Out, inv)

		return nil
	}

	var b bytes.Buffer
	printInvite(&b, inv, true)
	b.WriteTo(io.Out)

	return nil
}

func printInvite(w io.Writer, in *api.Invitation, headers bool) {
	if headers {
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "Org", "Email", "Redeemed")
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Fprintf(w, "%-20s %-20s %-10t\n", in.Organization.Slug, in.Email, in.Redeemed)
}
