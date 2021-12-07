package wireguard

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newList() (cmd *cobra.Command) {
	const (
		short = "List all WireGuard peer connections"
		long  = short + "\n"
	)

	cmd = command.New("list [-org ORG]", short, long, runList,
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

	peers, err := client.GetWireGuardPeers(ctx, org.Slug)
	if err != nil {
		return fmt.Errorf("failed retrieving wireguard peers: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if config.FromContext(ctx).JSONOutput {
		render.JSON(out, peers)

		return nil
	}

	var rows [][]string
	for _, peer := range peers {
		rows = append(rows, []string{peer.Name, peer.Region, peer.Peerip})
	}

	_ = render.Table(out, "", rows, "Name", "Region", "Peer IP")

	return nil
}
