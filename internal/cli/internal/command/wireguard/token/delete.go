package token

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newDelete() (cmd *cobra.Command) {
	const (
		short = "Delete a WireGuard token"
		long  = short + "\n"
		usage = "delete [-org ORG] [NAME]"
	)

	cmd = command.New(usage, short, long, runDelete,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.Org(),
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return
}

func runDelete(ctx context.Context) error {
	orgType := api.OrganizationTypeShared
	org, err := prompt.Org(ctx, &orgType)
	if err != nil {
		return err
	}

	name, err := nameFromFirstArgOrPrompt(ctx)
	if err != nil {
		return err
	}

	client := client.FromContext(ctx).API()

	if err := client.DeleteDelegatedWireGuardToken(ctx, org, &name, nil); err != nil {
		return fmt.Errorf("failed deleteting WireGuard token: %w", err)
	}

	if out := iostreams.FromContext(ctx).Out; !config.FromContext(ctx).JSONOutput {
		fmt.Fprintln(out, "token deleted.")
	}

	return nil
}
