package regions

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func v2RunRegionsList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	machineRegionsMap := make(map[string]map[string]bool)
	for _, machine := range machines {
		if machineRegionsMap[machine.Config.ProcessGroup()] == nil {
			machineRegionsMap[machine.Config.ProcessGroup()] = make(map[string]bool)
		}
		machineRegionsMap[machine.Config.ProcessGroup()][machine.Region] = true
	}

	machineRegions := make(map[string][]string)
	for group, regions := range machineRegionsMap {
		for region := range regions {
			machineRegions[group] = append(machineRegions[group], region)
		}
	}

	printApssV2Regions(ctx, machineRegions)
	return nil
}

type printableProcessGroup struct {
	Name    string
	Regions []string
}

func printApssV2Regions(ctx context.Context, machineRegions map[string][]string) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	if config.FromContext(ctx).JSONOutput {
		jsonPg := []printableProcessGroup{}
		for group, regionlist := range machineRegions {
			jsonPg = append(jsonPg, printableProcessGroup{
				Name:    group,
				Regions: regionlist,
			})
		}

		// only show pg if there's more than one
		data := struct {
			ProcessGroupRegions []printableProcessGroup
		}{
			ProcessGroupRegions: jsonPg,
		}
		render.JSON(io.Out, data)
		return
	}

	for group, regionlist := range machineRegions {
		fmt.Fprintf(io.Out, "Regions [%s]: %s\n", colorize.Bold(group), strings.Join(regionlist, ", "))
	}
}
