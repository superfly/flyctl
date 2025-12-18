package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newReplaySourcesList() *cobra.Command {
	const (
		long  = `List organizations allowed to replay requests to this organization.`
		short = "List allowed replay source organizations"
		usage = "list"
	)

	cmd := command.New(usage, short, long, runReplaySourcesList,
		command.RequireSession,
	)

	cmd.Aliases = []string{"ls"}
	flag.Add(cmd, flag.Org(), flag.JSONOutput())

	return cmd
}

func runReplaySourcesList(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	org, err := OrgFromFlagOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	sourceOrgSlugs, err := client.GetAllowedReplaySourceOrgSlugs(ctx, org.RawSlug)
	if err != nil {
		return fmt.Errorf("failed to get allowed replay source orgs: %w", err)
	}

	io := iostreams.FromContext(ctx)
	cfg := config.FromContext(ctx)

	if cfg.JSONOutput {
		return render.JSON(io.Out, sourceOrgSlugs)
	}

	if len(sourceOrgSlugs) == 0 {
		fmt.Fprintf(io.Out, "No replay source organizations configured for %s\n", org.RawSlug)
		return nil
	}

	fmt.Fprintf(io.Out, "Allowed replay source organizations for %s:\n", org.RawSlug)
	for _, slug := range sourceOrgSlugs {
		fmt.Fprintf(io.Out, "  %s\n", slug)
	}

	return nil
}
