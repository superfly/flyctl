package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
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

	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runCreate(ctx context.Context) error {
	name, err := nameFromFirstArgOrPrompt(ctx)
	if err != nil {
		return err
	}

	client := client.FromContext(ctx).API()

	org, err := client.CreateOrganization(ctx, name)
	if err != nil {
		return fmt.Errorf("failed creating organization: %w", err)
	}

	if io := iostreams.FromContext(ctx); config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, org)
	} else {
		printOrg(io.Out, org, true)
	}

	return nil
}

func nameFromFirstArgOrPrompt(ctx context.Context) (name string, err error) {
	if name = flag.FirstArg(ctx); name != "" {
		return
	}

	const msg = "Enter Organization Name:"

	if err = prompt.String(ctx, &name, msg, "", true); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("name argument must be specified when not running interactively")
	}

	return
}
