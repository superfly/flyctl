package lfsc

import (
	"context"
	"fmt"
	"slices"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/lfsc-go"
)

func newRegions() (cmd *cobra.Command) {
	const (
		long  = `View a list of LiteFS Cloud regions`
		short = "List LiteFS Cloud regions"
	)

	cmd = command.New("regions", short, long, runRegions,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		urlFlag(),
		flag.JSONOutput())

	return
}

func runRegions(ctx context.Context) error {
	flapsClient := flapsutil.ClientFromContext(ctx)

	regionData, err := flapsClient.GetRegions(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving regions: %w", err)
	}
	flyRegions := regionData.Regions

	// Filter out deprecated regions
	flyRegions = lo.Filter(flyRegions, func(r fly.Region, _ int) bool {
		return !r.Deprecated
	})

	lfscClient := lfsc.NewClient()
	lfscClient.URL = flag.GetString(ctx, "url")

	regions, err := lfscClient.Regions(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving litefs cloud regions: %w", err)
	}
	slices.Sort(regions)

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, regions)
	}

	var rows [][]string
	for _, region := range regions {
		var regionName string
		for _, flyRegion := range flyRegions {
			if flyRegion.Code == region {
				regionName = flyRegion.Name
			}
		}

		rows = append(rows, []string{regionName, region})
	}

	return render.Table(out, "", rows, "Name", "Code")
}
