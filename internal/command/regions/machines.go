package regions

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsv2"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"
)

func v2RunRegionsList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
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

func v2RunRegionsAdd(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	appConfig := appconfig.ConfigFromContext(ctx)

	// Compute current regions with machines
	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	// Process group to add
	groupName := ""
	processNames := appConfig.ProcessNames()
	if !slices.Contains(processNames, api.MachineProcessGroupApp) {
		// No app group found, so we require the process-group flag
		groupName = flag.GetString(ctx, "group")
		if groupName == "" {
			return fmt.Errorf("--group flag is required when no group named 'app' is defined")
		}
	}
	if groupName == "" {
		groupName = appConfig.DefaultProcessName()
	}

	// Compute regions missing machines
	machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
	if err != nil {
		return err
	}

	currentRegions := lo.CountValues(lo.Map(machines,
		func(m *api.Machine, _ int) string { return m.Region }),
	)
	missingRegions := lo.Uniq(lo.Filter(flag.Args(ctx),
		func(rn string, _ int) bool { return currentRegions[rn] == 0 }),
	)

	if len(missingRegions) == 0 {
		fmt.Fprintf(io.Out, "App already present in all requested regions\n")
		return nil
	}

	// Add one machine per region to the same process group
	groupCounts := map[string]int{
		groupName: len(missingRegions),
	}

	input := appsv2.ScaleCountInput{
		AppName:             appName,
		AppConfig:           appConfig,
		ExpectedGroupCounts: groupCounts,
		Regions:             missingRegions,
		AutoConfirm:         flag.GetYes(ctx),
		MaxPerRegion:        -1,
	}

	return appsv2.ScaleCount(ctx, input)
}
