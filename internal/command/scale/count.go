package scale

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
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
		failOnMachinesApp,
	)
	cmd.Args = cobra.MinimumNArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{Name: "max-per-region", Description: "Max number of VMs per region", Default: -1},
	)
	return cmd
}

func runScaleCount(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	appConfig := appconfig.ConfigFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	defaultGroupName := appConfig.DefaultProcessName()
	groups := map[string]int{}

	args := flag.Args(ctx)

	// single numeric arg: fly scale count 3
	if len(args) == 1 {
		count, err := strconv.Atoi(args[0])
		if err == nil {
			groups[defaultGroupName] = count
		}
	}

	// group labels: fly scale web=X worker=Y
	if len(groups) < 1 {
		for _, arg := range args {
			parts := strings.Split(arg, "=")
			if len(parts) != 2 {
				return fmt.Errorf("'%s' is not a valid process=count option", arg)
			}
			count, err := strconv.Atoi(parts[1])
			if err != nil {
				return err
			}

			groups[parts[0]] = count
		}
	}

	var maxPerRegion *int
	if v := flag.GetInt(ctx, "max-per-region"); v >= 0 {
		maxPerRegion = &v
	}

	counts, warnings, err := apiClient.SetAppVMCount(ctx, appName, groups, maxPerRegion)
	if err != nil {
		return err
	}

	if len(warnings) > 0 {
		for _, warning := range warnings {
			fmt.Fprintln(io.Out, "Warning:", warning)
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintf(io.Out, "Count changed to %s\n", countMessage(counts))
	return nil
}
