package orgs

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newDelete() *cobra.Command {
	const (
		long = `Delete an existing organization.
`
		short = "Delete an organization"
		usage = "delete [org]"
	)

	cmd := command.New(usage, short, long, runDelete,
		command.RequireSession)

	flag.Add(cmd,
		flag.Yes(),
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDelete(ctx context.Context) error {
	name, err := fetchSlug(ctx)
	if err != nil {
		return err
	}

	client := client.FromContext(ctx).API()

	org, err := client.GetOrganizationBySlug(ctx, name)
	if err != nil {
		return errors.Wrapf(err, "failed retrieving organization %s details", name)
	}

	io := iostreams.FromContext(ctx)
	if !flag.GetYes(ctx) {
		fmt.Fprintln(io.ErrOut, aurora.Red("Deleting an organization is not reversible."))

		msg := fmt.Sprintf("Delete organization %s?", name)
		if confirmed, err := prompt.Confirm(ctx, msg); err != nil || !confirmed {
			return err
		}
	}

	if _, err = client.DeleteOrganization(ctx, org.ID); err != nil {
		return errors.Wrapf(err, "failed deleting organization %s", err)
	}

	return nil
}
