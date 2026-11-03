package scale

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/completion"
)

func newScaleCount() *cobra.Command {
	const (
		short = "Change an app's VM count to the given value"
		long  = `Change an app's VM count to the given value.

The count is a total across the selected regions. With --per-region, it is
an absolute count in each explicitly selected region. For example:
  fly scale count 4 --per-region --region ord,sjc
sets four machines in ord and four in sjc, creating or destroying machines as needed.
--per-region cannot be combined with relative counts or --max-per-region.

For pricing, see https://fly.io/docs/about/pricing/`
	)
	cmd := command.New("count [count]", short, long, runScaleCount,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = validateScaleCountArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
		flag.ProcessGroup("The process group to scale"),
		flag.Bool{Name: "per-region", Description: "Set the absolute count in each region specified by --region"},
		flag.Int{Name: "max-per-region", Description: "Max number of VMs per region", Default: -1},
		flag.String{Name: "region", Shorthand: "r", Description: "Comma separated list of regions to act on. Defaults to all regions where there is at least one machine running for the app", CompletionFn: completion.CompleteRegions},
		flag.Bool{Name: "with-new-volumes", Description: "New machines each get a new volumes even if there are unattached volumes available"},
		flag.String{Name: "from-snapshot", Description: "New volumes are restored from snapshot, use 'last' for most recent snapshot. The default is an empty volume"},
		flag.VMSizeFlags,
		flag.Env(),
	)

	return cmd
}

func validateScaleCountArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		return err
	}
	ctx := flag.NewContext(context.Background(), cmd.Flags())
	if !flag.GetBool(ctx, "per-region") {
		return nil
	}
	if flag.IsSpecified(ctx, "max-per-region") {
		return fmt.Errorf("--per-region cannot be combined with --max-per-region")
	}
	for _, region := range strings.Split(flag.GetRegion(ctx), ",") {
		if strings.TrimSpace(region) == "" || region != strings.TrimSpace(region) {
			return fmt.Errorf("--per-region requires an explicit --region list without empty or whitespace-padded regions")
		}
	}
	groups, err := parseGroupCounts(args, fly.MachineProcessGroupApp)
	if err != nil {
		return err
	}
	for _, count := range groups {
		if count.relative != 0 {
			return fmt.Errorf("--per-region requires absolute counts")
		}
	}

	return nil
}

func runScaleCount(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

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

	groups, err := parseGroupCounts(args, groupName)
	if err != nil {
		return err
	}

	unknownNames := lo.Filter(slices.Collect(maps.Keys(groups)), func(x string, _ int) bool {
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

type groupCount struct{ absolute, relative int }
type groupCounts map[string]groupCount

func parseGroupCounts(args []string, defaultGroupName string) (groupCounts, error) {
	groups := make(groupCounts)
	apply := func(group string, countStr string) error {
		var count groupCount
		countNum, err := strconv.Atoi(countStr)
		if err != nil {
			return err
		}
		if strings.HasPrefix(countStr, "+") || strings.HasPrefix(countStr, "-") {
			if countNum == 0 {
				return fmt.Errorf("invalid relative scale count: %v", countStr)
			}
			count.relative = countNum
		} else {
			count.absolute = countNum
		}
		groups[group] = count

		return nil
	}

	// single numeric arg: fly scale count 3
	if len(args) == 1 {
		_ = apply(defaultGroupName, args[0]) // fall-through on error
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
