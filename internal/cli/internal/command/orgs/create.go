package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newCreate() *cobra.Command {
	const (
		long = `Create a new organization. Other users can be invited to join the 
organization later.
`
		short = "Create an organization"
		usage = "create [org]"
	)

	cmd := command.New(usage, short, long, runCreate,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var slug string
	if slug, err = fetchSlug(ctx); err != nil {
		return
	}

	client := client.FromContext(ctx).API()

	var org *api.Organization
	if org, err = client.CreateOrganization(ctx, slug); err != nil {
		err = fmt.Errorf("failed creating organization: %w", err)

		return
	}

	if io := iostreams.FromContext(ctx); config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, org)
	} else {
		printOrg(io.Out, org, true)
	}

	return
}
