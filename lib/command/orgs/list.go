package orgs

import (
	"bytes"
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/lib/command"
	"github.com/superfly/flyctl/lib/config"
	"github.com/superfly/flyctl/lib/flag"
	"github.com/superfly/flyctl/lib/flyutil"
	"github.com/superfly/flyctl/lib/render"
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

	for _, org := range orgs {
		printOrg(&b, &org, first)
		first = false
	}

	b.WriteTo(out)

	return nil
}
