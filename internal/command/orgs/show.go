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

	// A missing balance shouldn't hide the rest, such as when the API doesn't have the field yet.
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
			}{org, credit})
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
// without trial credits and other grants, or nil when the org's billing parent
// pays its bills.
func purchasedCreditBalance(ctx context.Context, client genq.Client, slug string) (*int, error) {
	_ = `# @genqlient
	query GetOrganizationCreditBalance($slug: String!) {
		organization(slug: $slug) {
			# @genqlient(pointer: true)
			purchasedCreditBalance
		}
	}
	`

	resp, err := gql.GetOrganizationCreditBalance(ctx, client, slug)
	if err != nil {
		return nil, err
	}

	return resp.Organization.PurchasedCreditBalance, nil
}

func formatCredit(cents *int) string {
	if cents == nil {
		return "billed to parent organization"
	}

	sign, c := "", *cents
	if c < 0 {
		sign, c = "-", -c
	}

	return fmt.Sprintf("%s$%s.%02d", sign, humanize.Comma(int64(c/100)), c%100)
}
