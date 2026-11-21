package orgs

import (
	"bytes"
	"context"
	"fmt"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runShow(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)
	selectedOrg, err := OrgFromEnvVarOrFirstArgOrSelect(ctx)
	if err != nil {
		return err
	}

	org, err := client.GetDetailedOrganizationBySlug(ctx, selectedOrg.Slug)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// A missing balance shouldn't hide the rest, such as after a network or API error.
	credit, creditErr := purchasedCreditBalance(ctx, client.GenqClient(), selectedOrg.Slug)
	if creditErr != nil {
		fmt.Fprintln(io.ErrOut, colorize.WarningIcon(), colorize.Yellow(fmt.Sprintf(
			"Couldn't get the purchased credit balance for organization %s: %v", selectedOrg.Slug, creditErr)))
	}

	if config.FromContext(ctx).JSONOutput {
		if creditErr != nil {
			_ = render.JSON(io.Out, org)
		} else {
			_ = render.JSON(io.Out, struct {
				*fly.OrganizationDetails
				PurchasedCreditBalanceCents *int
				BilledByParent              bool
			}{org, credit, credit == nil})
		}

		return nil
	}

	var buf bytes.Buffer

	fmt.Fprintln(&buf, colorize.Bold("Organization"))
	fmt.Fprintf(&buf, "%-16s: %-20s\n", "Name", org.Name)
	fmt.Fprintf(&buf, "%-16s: %-20s\n", "Slug", org.Slug)
	fmt.Fprintf(&buf, "%-16s: %-20s\n", "Type", org.Type)
	if creditErr == nil {
		fmt.Fprintf(&buf, "%-16s: %-20s\n", "Purchased credit", formatCredit(credit))
	}
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, colorize.Bold("Summary"))
	fmt.Fprintf(&buf, "You have %s permissions on this organization\n", org.ViewerRole)
	// fmt.Fprintf(&buf, "There are %d DNS zones associated with this organization\n", len(org.DNSZones.Nodes))
	memberCount := len(org.Members.Edges)
	verb, noun := "are", "members"
	if memberCount == 1 {
		verb, noun = "is", "member"
	}
	fmt.Fprintf(&buf, "There %s %d %s associated with this organization\n", verb, memberCount, noun)
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, colorize.Bold("Organization Members"))

	membertable := tablewriter.NewTable(&buf,
		tablewriter.WithHeader([]string{"Name", "Email", "Role"}),
	)

	for _, m := range org.Members.Edges {
		membertable.Append(m.Node.Name, m.Node.Email, m.Role) //nolint:errcheck
	}
	membertable.Render() //nolint:errcheck

	buf.WriteTo(io.Out)

	return nil
}

// purchasedCreditBalance returns the org's purchased credit balance in cents,
// without trial credits, or nil when a parent org pays for its
// usage. Money owed is a negative balance, which reads as 0, as on the billing page.
func purchasedCreditBalance(ctx context.Context, client genq.Client, slug string) (*int, error) {
	_ = `# @genqlient
	query GetOrganizationCreditBalance($slug: String!) {
		organization(slug: $slug) {
			creditBalance
			billedByParent
		}
	}
	`

	resp, err := gql.GetOrganizationCreditBalance(ctx, client, slug)
	if err != nil {
		return nil, err
	}
	if resp.Organization.BilledByParent {
		return nil, nil
	}

	cents := max(resp.Organization.CreditBalance, 0)

	return &cents, nil
}

func formatCredit(cents *int) string {
	if cents == nil {
		return "billed through parent organization"
	}

	return fmt.Sprintf("$%s.%02d", humanize.Comma(int64(*cents/100)), *cents%100)
}
