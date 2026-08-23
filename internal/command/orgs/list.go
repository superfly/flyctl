package orgs

import (
	"bytes"
	"context"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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

	flag.Add(cmd, flag.JSONOutput())
	cmd.Aliases = []string{"ls"}

	return cmd
}

func runList(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	orgs, err := client.GetOrganizations(ctx)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out

	if config.FromContext(ctx).JSONOutput {
		bySlug := map[string]string{}

		for _, other := range orgs {
			bySlug[other.Slug] = other.Name
		}

		_ = render.JSON(out, bySlug)

		return nil
	}

	var (
		b     bytes.Buffer
		first = true
	)
	table := tabwriter.NewWriter(&b, 21, 8, 1, ' ', 0)

	for _, org := range orgs {
		printOrg(table, &org, first)
		first = false
	}
	_ = table.Flush()

	b.WriteTo(out)

	return nil
}
