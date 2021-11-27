package orgs

import (
	"bytes"
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newShow() *cobra.Command {
	const (
		long = `Shows information about an organization.
Includes name, slug and type. Summarizes user permissions, DNS zones and
associated member. Details full list of members and roles.
`
		short = "Show information about an organization"
		usage = "show [org]"
	)

	cmd := command.New(usage, short, long, runShow,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runShow(ctx context.Context) error {
	slug, err := fetchSlug(ctx)
	if err != nil {
		return err
	}

	org, err := retrieveOrgBySlug(ctx, slug)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, org)

		return nil
	}

	var buf bytes.Buffer

	fmt.Fprintln(&buf, aurora.Bold("Organization"))
	fmt.Fprintf(&buf, "%-10s: %-20s\n", "Name", org.Name)
	fmt.Fprintf(&buf, "%-10s: %-20s\n", "Slug", org.Slug)
	fmt.Fprintf(&buf, "%-10s: %-20s\n", "Type", org.Type)
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, aurora.Bold("Summary"))
	fmt.Fprintf(&buf, "You have %s permissions on this organizaton\n", org.ViewerRole)
	// fmt.Fprintf(&buf, "There are %d DNS zones associated with this organization\n", len(org.DNSZones.Nodes))
	fmt.Fprintf(&buf, "There are %d members associated with this organization\n", len(org.Members.Edges))
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, aurora.Bold("Organization Members"))

	membertable := tablewriter.NewWriter(&buf)
	membertable.SetHeader([]string{"Name", "Email", "Role"})

	for _, m := range org.Members.Edges {
		membertable.Append([]string{m.Node.Name, m.Node.Email, m.Role})
	}
	membertable.Render()

	buf.WriteTo(io.Out)

	return nil
}
