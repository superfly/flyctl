package wireguard

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newList() *cobra.Command {
	const (
		short = "List all WireGuard peer connections"
		long  = short + "\n"
	)

	return command.New("list [org]", short, long, runList,
		command.RequireSession,
	)
}

func runList(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	slug, err := command.OrgSlugFromFirstArgOrSelect(ctx)
	if err != nil {
		return err
	}

	peers, err := client.GetWireGuardPeers(ctx, slug)
	if err != nil {
		return err
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

	render.Table(out, "", rows, "Name", "Region", "Peer IP")

	return nil
}
