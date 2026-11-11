package orgs

import (
	"bytes"
	"context"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		long = `List the organizations you belong to, with each one's name, slug and type
(personal or shared). Use the slug with ` + "`--org`" + ` in other commands.

With ` + "`--json`" + `, prints an object that maps slugs to names.`
		short = "List your organizations"
	)

	cmd := command.New("list", short, long, runList,
		command.RequireSession,
	)

	flag.Add(cmd, flag.JSONOutput())
	cmd.Aliases = []string{"ls"}

	return cmd
}

func runList(ctx context.Context) error {
	orgs, err := uiexutil.ClientFromContext(ctx).ListOrganizations(ctx, false)
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
	table := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	for _, org := range orgs {
		printOrg(table, &org, first)
		first = false
	}
	_ = table.Flush()

	b.WriteTo(out)

	return nil
}
