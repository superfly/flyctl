package platform

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newRegions() (cmd *cobra.Command) {
	const (
		long = `View a list of regions where Fly has edges and/or datacenters
`
		short = "List regions"
	)

	cmd = command.New("regions", short, long, runRegions,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs
	flag.Add(cmd, flag.JSONOutput())
	return
}

func runRegions(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	regions, _, err := client.PlatformRegions(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving regions: %w", err)
	}
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Name < regions[j].Name
	})

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, regions)
	}

	var rows [][]string
	for _, region := range regions {
		gateway := ""
		if region.GatewayAvailable {
			gateway = "✓"
		}
		paidPlan := ""
		if region.RequiresPaidPlan {
			paidPlan = "✓"
		}
		rows = append(rows, []string{
			region.Name,
			region.Code,
			gateway,
			paidPlan,
		})
	}

	return render.Table(out, "", rows, "Name", "Code", "Gateway", "Paid Plan Only")
}
