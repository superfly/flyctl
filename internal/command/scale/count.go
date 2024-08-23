package scale

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flapsutil"
	"golang.org/x/exp/maps"
)

func newScaleCount() *cobra.Command {
	const (
		short = "Change an app's VM count to the given value"
		long  = `Change an app's VM count to the given value.

For pricing, see https://fly.io/docs/about/pricing/`
	)
	cmd := command.New("count [count]", short, long, runScaleCount,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MinimumNArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
		flag.ProcessGroup("The process group to scale"),
		flag.Int{Name: "max-per-region", Description: "Max number of VMs per region", Default: -1},
		flag.String{Name: "region", Shorthand: "r", Description: "Comma separated list of regions to act on. Defaults to all regions where there is at least one machine running for the app", CompletionFn: completion.CompleteRegions},
		flag.Bool{Name: "with-new-volumes", Description: "New machines each get a new volumes even if there are unattached volumes available"},
		flag.String{Name: "from-snapshot", Description: "New volumes are restored from snapshot, use 'last' for most recent snapshot. The default is an empty volume"},
		flag.VMSizeFlags,
	)
	return cmd
}

func runScaleCount(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	appConfig, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	args := flag.Args(ctx)

	processNames := appConfig.ProcessNames()
	groupName := flag.GetProcessGroup(ctx)

	if groupName == "" {
		groupName = fly.MachineProcessGroupApp
	}

	groups, err := parseGroupCounts(ctx, args, groupName)
	if err != nil {
		return err
	}

	unknownNames := lo.Filter(maps.Keys(groups), func(x string, _ int) bool {
		return !slices.Contains(processNames, x)
	})
	if len(unknownNames) > 0 {
		return fmt.Errorf(
			"Attempting to scale unknown process groups %v but valid names are %v.\n"+
				" Use `fly scale count COUNT --process-group=NAME` \n"+
				" or multi group syntax `fly scale count NAME1=COUNT1 NAME2=COUNT2 ...`",
			unknownNames,
			processNames,
		)
	}

	maxPerRegion := flag.GetInt(ctx, "max-per-region")

	return runMachinesScaleCount(ctx, appName, appConfig, groups, maxPerRegion)
}

func parseGroupCounts(ctx context.Context, args []string, defaultGroupName string) (map[string]int, error) {
	groups := make(map[string]int)

	var machineGroups map[string][]*fly.Machine

	apply := func(group string, count string) error {
		delta := 0
		if strings.HasPrefix(count, "+") || strings.HasPrefix(count, "-") {
			if machineGroups == nil {
				flapsClient := flapsutil.ClientFromContext(ctx)
				machines, _, err := flapsClient.ListFlyAppsMachines(ctx)
				if err != nil {
					return err
				}
				machineGroups = lo.GroupBy(machines, func(m *fly.Machine) string {
					return m.ProcessGroup()
				})
			}
			delta = len(machineGroups[group])
		}
		countNum, err := strconv.Atoi(count)
		if err != nil {
			return err
		}
		groups[group] = countNum + delta
		return nil
	}

	// single numeric arg: fly scale count 3
	if len(args) == 1 {
		if err := apply(defaultGroupName, args[0]); err != nil {
			return nil, err
		}
	}

	// group labels: fly scale count web=X worker=Y
	if len(groups) < 1 {
		for _, arg := range args {
			parts := strings.Split(arg, "=")
			if len(parts) != 2 {
				return nil, fmt.Errorf("'%s' is not a valid process=count option", arg)
			}
			if err := apply(parts[0], parts[1]); err != nil {
				return nil, err
			}
		}
	}

	return groups, nil
}
