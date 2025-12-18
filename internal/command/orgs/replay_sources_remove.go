package orgs

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newReplaySourcesRemove() *cobra.Command {
	const (
		long = `Remove organizations from the list of allowed replay sources for this organization.

If no slugs are provided, an interactive selector will be shown.`
		short = "Remove allowed replay source organizations"
		usage = "remove [<slug>...]"
	)

	cmd := command.New(usage, short, long, runReplaySourcesRemove,
		command.RequireSession,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(cmd,
		flag.Org(),
		flag.Yes(),
	)

	return cmd
}

func runReplaySourcesRemove(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	org, err := OrgFromFlagOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	userOrgs, err := client.GetOrganizations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get user organizations: %w", err)
	}

	userOrgMap := make(map[string]string)
	for _, userOrg := range userOrgs {
		userOrgMap[userOrg.RawSlug] = userOrg.Name
	}

	args := flag.Args(ctx)
	var orgSlugsToRemove []string

	if len(args) == 0 {
		// Interactive mode: show multi-select of currently allowed orgs
		currentAllowed, err := client.GetAllowedReplaySourceOrgSlugs(ctx, org.RawSlug)
		if err != nil {
			return fmt.Errorf("failed to get current replay sources: %w", err)
		}

		if len(currentAllowed) == 0 {
			fmt.Fprintln(io.Out, "No replay source organizations configured")
			return nil
		}

		var options []string
		for _, slug := range currentAllowed {
			if name, isMember := userOrgMap[slug]; isMember {
				options = append(options, fmt.Sprintf("%s (%s)", name, slug))
			} else {
				options = append(options, fmt.Sprintf("%s (not a member - cannot re-add)", slug))
			}
		}

		var selections []int
		if err := prompt.MultiSelect(ctx, &selections, "Select organizations to remove:", nil, options...); err != nil {
			return err
		}

		if len(selections) == 0 {
			return nil
		}

		for _, idx := range selections {
			orgSlugsToRemove = append(orgSlugsToRemove, currentAllowed[idx])
		}
	} else {
		// Use positional arguments
		orgSlugsToRemove = args
	}

	// Check which orgs user is NOT a member of
	var nonMemberSlugs []string
	for _, slug := range orgSlugsToRemove {
		if _, isMember := userOrgMap[slug]; !isMember {
			nonMemberSlugs = append(nonMemberSlugs, slug)
		}
	}

	// Warn about non-member orgs if not using --yes
	if len(nonMemberSlugs) > 0 && !flag.GetYes(ctx) {
		fmt.Fprintf(io.Out, "Warning: You are not a member of: %s\n", strings.Join(nonMemberSlugs, ", "))
		fmt.Fprintf(io.Out, "You will not be able to re-add these organizations.\n")

		confirmed, err := prompt.Confirm(ctx, "Continue with removal?")
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	_, err = client.RemoveAllowedReplaySourceOrgs(ctx, org.RawSlug, orgSlugsToRemove)
	if err != nil {
		return fmt.Errorf("failed to remove allowed replay source orgs: %w", err)
	}

	fmt.Fprintf(io.Out, "Removed allowed replay source organizations from %s: %s\n",
		org.RawSlug, strings.Join(orgSlugsToRemove, ", "))

	return nil
}
