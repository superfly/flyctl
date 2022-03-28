package orgs

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/sort"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
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

func slugFromFirstArgOrSelect(ctx context.Context) (slug string, err error) {
	if slug = flag.FirstArg(ctx); slug != "" {
		return
	}

	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		err = errSlugArgMustBeSpecified

		return
	}

	client := client.FromContext(ctx).API()

	typ := api.OrganizationTypeShared

	var orgs []api.Organization
	if orgs, err = client.GetOrganizations(ctx, &typ); err != nil {
		return
	}
	sort.OrganizationsByTypeAndName(orgs)

	var org *api.Organization
	if org, err = prompt.SelectOrg(ctx, orgs); prompt.IsNonInteractive(err) {
		err = errSlugArgMustBeSpecified
	} else {
		slug = org.Slug
	}

	return
}

func detailsFromFirstArgOrSelect(ctx context.Context) (*api.OrganizationDetails, error) {
	slug, err := slugFromFirstArgOrSelect(ctx)
	if err != nil {
		return nil, err
	}

	return detailsFromSlug(ctx, slug)
}

func detailsFromSlug(ctx context.Context, slug string) (*api.OrganizationDetails, error) {
	client := client.FromContext(ctx).API()

	org, err := client.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving organization with slug %s: %w", slug, err)
	}

	return org, nil
}

func printOrg(w io.Writer, org *api.Organization, headers bool) {
	if headers {
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "Name", "Slug", "Type")
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Fprintf(w, "%-20s %-20s %-10s\n", org.Name, org.Slug, org.Type)
}
