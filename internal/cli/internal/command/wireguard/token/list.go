package token

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newList() (cmd *cobra.Command) {
	const (
		short = "List WireGuard tokens"
		long  = short + "\n"
		usage = "list [-org ORG]"
	)

	cmd = command.New(usage, short, long, runList,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.Org(),
	)

	return
}

func runList(ctx context.Context) error {
	orgType := api.OrganizationTypeShared
	org, err := prompt.Org(ctx, &orgType)
	if err != nil {
		return err
	}

	client := client.FromContext(ctx).API()

	tokens, err := client.GetDelegatedWireGuardTokens(ctx, org.Slug)
	if err != nil {
		return fmt.Errorf("failed retrieving wireguard tokens: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if config.FromContext(ctx).JSONOutput {
		_ = render.JSON(out, tokens)

		return nil
	}

	var rows [][]string
	for _, token := range tokens {
		rows = append(rows, []string{token.Name})
	}

	_ = render.Table(out, "", rows, "Name")

	return nil
}
