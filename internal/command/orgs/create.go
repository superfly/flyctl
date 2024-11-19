package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newCreate() *cobra.Command {
	const (
		long = `Create a new organization. Other users can be invited to join the
organization later.
`
		short = "Create an organization"
		usage = "create [name]"
	)

	cmd := command.New(usage, short, long, runCreate,
		command.RequireSession)

	flag.Add(cmd,
		flag.Bool{
			Name:        "apps-v2-default-on",
			Description: "Configure this org to use apps v2 by default for new apps (deprecated)",
			Default:     false,
			Hidden:      true,
		},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runCreate(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return err
	}

	var name string

	name = flag.FirstArg(ctx)

	if user.EnablePaidHobby {
		fmt.Fprintf(io.Out, "New organizations start on the Pay As You Go plan.\n\n")

		if name == "" {
			confirmed, err := prompt.Confirm(ctx, "Do you still want to create the organization?")
			if err != nil {
				return err
			}

			if !confirmed {
				return nil
			}
		}
	}

	if name == "" {
		const msg = "Enter Organization Name:"

		if err = prompt.String(ctx, &name, msg, "", true); prompt.IsNonInteractive(err) {
			err = prompt.NonInteractiveError("name argument must be specified when not running interactively")
		}

		if err != nil {
			return err
		}

	}

	org, err := client.CreateOrganization(ctx, name)
	if err != nil {
		return fmt.Errorf("failed creating organization: %w", err)
	}

	if io := iostreams.FromContext(ctx); config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, org)
	} else {
		fmt.Fprintf(io.Out, "Your organization %s (%s) was created successfully. Visit %s to add a credit card and enable deployment.\n", org.Name, org.Slug, colorize.Green(fmt.Sprintf("https://fly.io/dashboard/%s/billing", org.Slug)))
	}

	return nil
}
