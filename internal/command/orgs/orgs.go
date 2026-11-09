package orgs

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/sort"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `Commands for managing Fly organizations. list, create, show and
destroy organizations.
Organization admins can also invite or remove users from Organizations.
`
		short = "Commands for managing Fly organizations"
	)

	// TODO: list should also accept the --org param

	orgs := command.New("orgs", short, long, nil)

	orgs.AddCommand(
		newList(),
		newShow(),
		newInvite(),
		newRemove(),
		newCreate(),
		newDelete(),
		newReplaySources(),
		newCrossNetworkReplays(),
	)

	return orgs
}

func emailFromSecondArgOrPrompt(ctx context.Context) (email string, err error) {
	if args := flag.Args(ctx); len(args) > 1 {
		email = args[1]

		return
	}

	const msg = "Enter User Email:"

	if err = prompt.String(ctx, &email, msg, "", true); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("email argument must be specified when not running interactively")
	}

	return
}

var errSlugArgMustBeSpecified = prompt.NonInteractiveError("slug argument must be specified when not running interactively")

// Filter narrows the organizations offered when prompting for one.
type Filter int

const (
	// AdminOnly limits the choice to organizations the user administers.
	AdminOnly Filter = iota
)

func slugFromArgOrSelect(ctx context.Context, orgSlug string, filters ...Filter) (slug string, err error) {
	if orgSlug != "" {
		return orgSlug, nil
	}

	if args := flag.Args(ctx); len(args) > 0 {
		return args[0], nil
	}

	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		err = errSlugArgMustBeSpecified

		return
	}

	adminOnly := slices.Contains(filters, AdminOnly)

	var orgs []uiex.Organization
	if orgs, err = uiexutil.ClientFromContext(ctx).ListOrganizations(ctx, adminOnly); err != nil {
		return
	}
	sort.OrganizationsByTypeAndName(orgs)

	var org *uiex.Organization
	if org, err = prompt.SelectOrg(ctx, orgs); prompt.IsNonInteractive(err) {
		err = errSlugArgMustBeSpecified
	} else if err == nil {
		slug = org.Slug
	}

	return
}

func OrgFromEnvVarOrFirstArgOrSelect(ctx context.Context, filters ...Filter) (*uiex.Organization, error) {
	slug := flag.GetOrg(ctx)
	if slug == "" {
		var err error
		slug, err = slugFromArgOrSelect(ctx, slug, filters...)
		if err != nil {
			return nil, err
		}
	}

	return OrgFromSlug(ctx, slug)
}

func OrgFromFlagOrSelect(ctx context.Context, filters ...Filter) (*uiex.Organization, error) {
	slug, err := slugFromArgOrSelect(ctx, flag.GetOrg(ctx), filters...)
	if err != nil {
		return nil, err
	}

	return OrgFromSlug(ctx, slug)
}

func OrgFromSlug(ctx context.Context, slug string) (*uiex.Organization, error) {
	org, err := uiexutil.ClientFromContext(ctx).GetOrganization(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving organization with slug %s: %w", slug, err)
	}

	return org, nil
}

func printOrg(w io.Writer, org *uiex.Organization, headers bool) {
	if headers {
		fmt.Fprintln(w, "Name\tSlug\tType")
		fmt.Fprintln(w, "----\t----\t----")
	}

	orgType := "SHARED"
	if org.Personal {
		orgType = "PERSONAL"
	}

	fmt.Fprintf(w, "%s\t%s\t%s\n", org.Name, org.Slug, orgType)
}
