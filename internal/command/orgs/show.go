package orgs

import (
	"bytes"
	"context"
	"fmt"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newShow() *cobra.Command {
	const (
		long = `Shows information about an organization.
Includes name, slug and type. Summarizes user permissions, DNS zones and
associated member. Details full list of members and roles.
`
		short = "Show information about an organization"
		usage = "show [slug]"
	)

	cmd := command.New(usage, short, long, runShow,
		command.RequireSession,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runShow(ctx context.Context) error {
	org, err := detailsFromFirstArgOrSelect(ctx)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, org)

		return nil
	}
	colorize := io.ColorScheme()

	var buf bytes.Buffer

	fmt.Fprintln(&buf, colorize.Bold("Organization"))
	fmt.Fprintf(&buf, "%-10s: %-20s\n", "Name", org.Name)
	fmt.Fprintf(&buf, "%-10s: %-20s\n", "Slug", org.Slug)
	fmt.Fprintf(&buf, "%-10s: %-20s\n", "Type", org.Type)
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, colorize.Bold("Summary"))
	fmt.Fprintf(&buf, "You have %s permissions on this organizaton\n", org.ViewerRole)
	// fmt.Fprintf(&buf, "There are %d DNS zones associated with this organization\n", len(org.DNSZones.Nodes))
	fmt.Fprintf(&buf, "There are %d members associated with this organization\n", len(org.Members.Edges))
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, colorize.Bold("Organization Members"))

	membertable := tablewriter.NewWriter(&buf)
	membertable.SetHeader([]string{"Name", "Email", "Role"})

	for _, m := range org.Members.Edges {
		membertable.Append([]string{m.Node.Name, m.Node.Email, m.Role})
	}
	membertable.Render()

	buf.WriteTo(io.Out)

	return nil
}
