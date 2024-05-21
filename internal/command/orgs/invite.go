package orgs

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newInvite() *cobra.Command {
	const (
		long = `Invite a user, by email, to join organization. The invitation will be
sent, and the user will be pending until they respond.
`
		short = "Invite user (by email) to organization"
		usage = "invite [slug] [email]"
	)

	cmd := command.New(usage, short, long, runInvite,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runInvite(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	org, err := OrgFromEnvVarOrFirstArgOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return nil
	}

	email, err := emailFromSecondArgOrPrompt(ctx)
	if err != nil {
		return nil
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

func printInvite(w io.Writer, in *fly.Invitation, headers bool) {
	if headers {
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "Org", "Email", "Redeemed")
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Fprintf(w, "%-20s %-20s %-10t\n", in.Organization.Slug, in.Email, in.Redeemed)
}
