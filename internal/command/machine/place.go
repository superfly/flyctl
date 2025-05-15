package machine

import (
	"context"
	"fmt"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newPlace() (cmd *cobra.Command) {
	const (
		long = `Simulate a batch of Machine placements across multiple regions
`
		short = "Simulate Machine placements"
	)

	cmd = command.New("place", short, long, runPlace,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.Org(),
		flag.VMSizeFlags,
		flag.Int{
			Name:        "count",
			Description: "number of machines to place",
			Default:     1,
		},
		flag.StringArray{
			Name:        "regions",
			Description: "list of regions to place machines",
		},
		flag.String{Name: "volume-name", Description: "name of the volume to place machines"},
		flag.String{Name: "desired-region", Description: "name of the desired region to place machines"},
		flag.Int{Name: "volume-size", Description: "size of the desired volume to place machines"},
	)
	return
}

func runPlace(ctx context.Context) error {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{})
	if err != nil {
		return err
	}
	vm, err := flag.GetMachineGuest(ctx, nil)
	if err != nil {
		return err
	}

	orgSlug := flag.GetOrg(ctx)
	if orgSlug == "" {
		appName := appconfig.NameFromContext(ctx)
		var org *fly.Organization
		if appName == "" {
			org, err = orgs.OrgFromFlagOrSelect(ctx)
		} else {
			org, err = flyutil.ClientFromContext(ctx).GetOrganizationByApp(ctx, appName)
		}
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	}

	regions, err := flapsClient.GetPlacements(ctx, &flaps.GetPlacementsRequest{
		VM:              vm,
		DesiredRegion:   flag.GetString(ctx, "desired-region"),
		Regions:         flag.GetStringArray(ctx, "regions"),
		Count:           int64(max(flag.GetInt(ctx, "count"), 1)),
		VolumeName:      flag.GetString(ctx, "volume-name"),
		VolumeSizeBytes: uint64(flag.GetInt(ctx, "volume-size") * units.GB),
		Weights:         nil,
		Size:            "",
		Org:             orgSlug,
	})
	if err != nil {
		return fmt.Errorf("failed getting machine placements: %w", err)
	}

	io := iostreams.FromContext(ctx)
	out := io.Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, regions)
	}

	var rows [][]string
	showConcurrency := false
	for _, region := range regions {
		count := fmt.Sprint(region.Count)
		if region.Concurrency != region.Count {
			showConcurrency = true
			count += fmt.Sprintf(" (%d)", region.Concurrency)
		}
		rows = append(rows, []string{
			region.Region,
			count,
		})
	}

	countCol := "Count"
	if showConcurrency {
		countCol += " (Concurrency)"
	}
	return render.Table(out, "", rows, "Region", countCol)
}
