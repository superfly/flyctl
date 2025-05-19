package machine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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
		},
		flag.String{
			Name:        "region",
			Description: "comma-delimited list of regions to place machines",
		},
		flag.String{Name: "volume-name", Description: "name of the volume to place machines"},
		flag.Int{Name: "volume-size", Description: "size of the desired volume to place machines"},
		flag.StringSlice{
			Name:        "weights",
			Description: "comma-delimited list of key=value weights to adjust placement preferences. e.g., 'region=5,spread=10'",
		},
	)
	return
}

func runPlace(ctx context.Context) error {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{})
	if err != nil {
		return err
	}
	guest := &fly.MachineGuest{}
	err = guest.SetSize("performance-1x")
	if err != nil {
		return err
	}
	vm, err := flag.GetMachineGuest(ctx, guest)
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

	weights, err := getWeights(ctx)
	if err != nil {
		return err
	}
	regions, err := flapsClient.GetPlacements(ctx, &flaps.GetPlacementsRequest{
		VM:              vm,
		Region:          flag.GetString(ctx, "region"),
		Count:           int64(flag.GetInt(ctx, "count")),
		VolumeName:      flag.GetString(ctx, "volume-name"),
		VolumeSizeBytes: uint64(flag.GetInt(ctx, "volume-size") * units.GB),
		Weights:         weights,
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
		row := []string{region.Region, count}
		if region.Concurrency != region.Count && region.Concurrency > 0 {
			showConcurrency = true
			row = append(row, fmt.Sprint(region.Concurrency))
		}
		rows = append(rows, row)
	}

	cols := []string{"Region", "Count"}
	if showConcurrency {
		cols = append(cols, "Concurrency")
	}

	return render.Table(out, "", rows, cols...)
}

func getWeights(ctx context.Context) (*flaps.Weights, error) {
	weightStr := flag.GetStringSlice(ctx, "weights")
	if len(weightStr) == 0 {
		return nil, nil
	}
	weights := make(flaps.Weights)
	for _, weight := range weightStr {
		parts := strings.SplitN(weight, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid weight: %q", weight)
		}
		w, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid weight: %q", weight)
		}
		weights[parts[0]] = w
	}
	return &weights, nil
}
