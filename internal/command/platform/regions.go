package platform

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"golang.org/x/exp/slices"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

// Hardcoded list of regions with GPUs
// TODO: fetch this list from the graphql endpoint once it is there
var gpuRegions = []string{"iad", "sjc", "syd", "ams"}

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
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{})
	if err != nil {
		return err
	}
	regions, err := flapsClient.GetRegions(ctx)
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
		gpuAvailable := ""
		if slices.Contains(gpuRegions, region.Code) {
			gpuAvailable = "✓"
		}

		io := iostreams.FromContext(ctx)
		colorize := io.ColorScheme()

		var capacity string
		switch c := region.Capacity; {
		case c == 0:
			capacity = colorize.Red("X")
		case c < 100:
			capacity = colorize.Magenta("▏")
		case c < 400:
			capacity = colorize.Yellow("▎")
		case c < 800:
			capacity = colorize.Green("▍")
		case c < 1000:
			capacity = colorize.Green("▌")
		default:
			capacity = colorize.Green("█")
		}

		rows = append(rows, []string{
			region.Name,
			region.Code,
			gateway,
			paidPlan,
			gpuAvailable,
			capacity,
		})
	}

	return render.Table(out, "", rows, "Name", "Code", "Gateway", "Launch Plan+", "GPUs", "Capacity")
}
