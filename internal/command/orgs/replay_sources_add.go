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

func newReplaySourcesAdd() *cobra.Command {
	const (
		long = `Add organizations to the list of allowed replay sources for this organization.

If no slugs are provided, an interactive selector will be shown.`
		short = "Add allowed replay source organizations"
		usage = "add [<slug>...]"
	)

	cmd := command.New(usage, short, long, runReplaySourcesAdd,
		command.RequireSession,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runReplaySourcesAdd(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	org, err := OrgFromFlagOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	args := flag.Args(ctx)
	var sourceOrgSlugs []string

	if len(args) == 0 {
		// Interactive mode: show multi-select of available orgs
		userOrgs, err := client.GetOrganizations(ctx)
		if err != nil {
			return fmt.Errorf("failed to get organizations: %w", err)
		}

		// Get currently allowed orgs to exclude them
		currentAllowed, err := client.GetAllowedReplaySourceOrgSlugs(ctx, org.RawSlug)
		if err != nil {
			return fmt.Errorf("failed to get current replay sources: %w", err)
		}
		currentSet := make(map[string]bool)
		for _, slug := range currentAllowed {
			currentSet[slug] = true
		}

		var options []string
		var availableSlugs []string
		for _, userOrg := range userOrgs {
			if userOrg.RawSlug != org.RawSlug && !currentSet[userOrg.RawSlug] {
				options = append(options, fmt.Sprintf("%s (%s)", userOrg.Name, userOrg.RawSlug))
				availableSlugs = append(availableSlugs, userOrg.RawSlug)
			}
		}

		if len(options) == 0 {
			fmt.Fprintln(io.Out, "No organizations available to add")
			return nil
		}

		var selections []int
		if err := prompt.MultiSelect(ctx, &selections, "Select organizations to add:", nil, options...); err != nil {
			return err
		}

		if len(selections) == 0 {
			return nil
		}

		for _, idx := range selections {
			sourceOrgSlugs = append(sourceOrgSlugs, availableSlugs[idx])
		}
	} else {
		// Use positional arguments
		sourceOrgSlugs = args
	}

	_, err = client.AddAllowedReplaySourceOrgs(ctx, org.RawSlug, sourceOrgSlugs)
	if err != nil {
		return fmt.Errorf("failed to add allowed replay source orgs: %w", err)
	}

	fmt.Fprintf(io.Out, "Added allowed replay source organizations for %s: %s\n",
		org.RawSlug, strings.Join(sourceOrgSlugs, ", "))

	return nil
}
