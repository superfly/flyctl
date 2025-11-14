package platform

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

const RegionsCommandDesc = `View a list of regions where Fly has datacenters.
'Capacity' shows how many performance-1x VMs can currently be launched in each region.
`

func newRegions() (cmd *cobra.Command) {
	const (
		short = "List regions"
	)

	cmd = command.New("regions", short, RegionsCommandDesc, runRegions,
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
	regions, err := flapsClient.GetRegions(ctx, "")
	if err != nil {
		return fmt.Errorf("failed retrieving regions: %w", err)
	}

	// Filter out deprecated regions
	regions = lo.Filter(regions, func(r fly.Region, _ int) bool {
		return !r.Deprecated
	})

	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Name < regions[j].Name
	})

	io := iostreams.FromContext(ctx)
	out := io.Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, regions)
	}

	var rows [][]string
	regionGroups := lo.GroupBy(regions, func(item fly.Region) fly.GeoRegion { return item.GeoRegion })
	keys := lo.Keys(regionGroups)
	slices.SortFunc(keys, func(a, b fly.GeoRegion) int { return cmp.Compare(a, b) })
	for _, key := range keys {
		regionGroup := regionGroups[key]
		rows = append(rows, []string{""})
		rows = append(rows, []string{io.ColorScheme().Underline(key.String())})
		for _, region := range regionGroup {
			capacity := fmt.Sprint(region.Capacity)
			capacity = io.ColorScheme().RedGreenGradient(capacity, float64(region.Capacity)/1000)

			rows = append(rows, []string{
				region.Name,
				region.Code,
				capacity,
			})
		}
	}

	return render.Table(out, "", rows, "Name", "Code", "Capacity")
}
