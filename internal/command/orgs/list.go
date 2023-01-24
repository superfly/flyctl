package orgs

import (
	"bytes"
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		long = `Lists organizations available to current user.
`
		short = "Lists organizations for current user"
	)

	cmd := command.New("list", short, long, runList,
		command.RequireSession,
	)

	cmd.Aliases = []string{"ls"}
	return cmd
}

func runList(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	personal, others, err := client.GetCurrentOrganizations(ctx)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out

	if config.FromContext(ctx).JSONOutput {
		orgs := map[string]string{
			personal.Slug: personal.Name,
		}

		for _, other := range others {
			orgs[other.Slug] = other.Name
		}

		_ = render.JSON(out, orgs)

		return nil
	}

	var b bytes.Buffer

	printOrg(&b, &personal, true)
	for _, org := range others {
		if org.ID != personal.ID {
			printOrg(&b, &org, false)
		}
	}

	b.WriteTo(out)

	return nil
}
