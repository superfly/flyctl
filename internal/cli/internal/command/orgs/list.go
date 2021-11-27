package orgs

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newList() *cobra.Command {
	const (
		long = `Lists organizations available to current user.
`
		short = "Lists organizations for current user"
	)

	return command.New("list", short, long, runList,
		command.RequireSession,
	)
}

func runList(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	personal, others, err := client.GetCurrentOrganizations(ctx)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out

	if config.FromContext(ctx).JSONOutput {
		orgs := struct {
			PersonalOrganization api.Organization
			Organizations        []api.Organization
		}{
			PersonalOrganization: personal,
			Organizations:        others,
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

func printOrg(w io.Writer, org *api.Organization, headers bool) {
	if headers {
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "Name", "Slug", "Type")
		fmt.Fprintf(w, "%-20s %-20s %-10s\n", "----", "----", "----")
	}

	fmt.Fprintf(w, "%-20s %-20s %-10s\n", org.Name, org.Slug, org.Type)
}
