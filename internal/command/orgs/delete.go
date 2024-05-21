package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newDelete() *cobra.Command {
	const (
		long = `Delete an existing organization.
`
		short = "Delete an organization"
		usage = "delete [-yes] [slug]"
	)

	cmd := command.New(usage, short, long, runDelete,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.Yes(),
	)

	return cmd
}

func runDelete(ctx context.Context) error {
	org, err := OrgFromEnvVarOrFirstArgOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	if !flag.GetYes(ctx) {
		const msg = "Deleting an organization is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Delete organization %s?", org.Slug); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	client := flyutil.ClientFromContext(ctx)
	if _, err := client.DeleteOrganization(ctx, org.ID); err != nil {
		return fmt.Errorf("failed deleting organization %s", err)
	}

	return nil
}
