package autoscale

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "V1 APPS ONLY: Autoscaling app resources"
		long  = `V1 APPS ONLY: Autoscaling application resources`
	)
	cmd := command.New("autoscale", short, long, nil)
	cmd.AddCommand(
		newAutoscaleDisable(),
		newAutoscaleSet(),
		newAutoscaleShow(),
	)
	return cmd
}

func newAutoscaleDisable() *cobra.Command {
	const (
		short = "V1 APPS ONLY: Disable autoscaling"
		long  = `V1 APPS ONLY: Disable autoscaling to manually control app resources`
	)
	cmd := command.New("disable", short, long, runAutoscaleDisable,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.MaximumNArgs(2)
	return cmd
}

func newAutoscaleSet() *cobra.Command {
	const (
		short = "Set app autoscaling parameters"
		long  = `V1 APPS ONLY: Enable autoscaling and set the application's autoscaling parameters:

min=int - minimum number of instances to be allocated globally.
max=int - maximum number of instances to be allocated globally.`
	)
	cmd := command.New("set", short, long, runAutoscaleSet,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.MaximumNArgs(2)
	return cmd
}

func newAutoscaleShow() *cobra.Command {
	const (
		short = "V1 APPS ONLY: Show current autoscaling configuration"
		long  = `V1 APPS ONLY: Show current autoscaling configuration`
	)
	cmd := command.New("show", short, long, runAutoscaleShow,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func runAutoscaleSet(ctx context.Context) error {
	return actualScale(ctx, false)
}

func runAutoscaleDisable(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if app.PlatformVersion == appconfig.MachinesPlatform {
		printMachinesAutoscalingBanner()
		return nil
	}

	newcfg := api.UpdateAutoscaleConfigInput{
		AppID:   appName,
		Enabled: api.BoolPointer(false),
	}

	cfg, err := apiClient.UpdateAutoscaleConfig(ctx, newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(ctx, cfg)

	return nil
}

func actualScale(ctx context.Context, balanceRegions bool) error {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if app.PlatformVersion == appconfig.MachinesPlatform {
		printMachinesAutoscalingBanner()
		return nil
	}

	currentcfg, err := apiClient.AppAutoscalingConfig(ctx, appName)
	if err != nil {
		return err
	}

	newcfg := api.UpdateAutoscaleConfigInput{
		AppID: appName,
	}

	newcfg.BalanceRegions = &balanceRegions
	newcfg.MinCount = &currentcfg.MinCount
	newcfg.MaxCount = &currentcfg.MaxCount

	kvargs := make(map[string]string)

	args := flag.Args(ctx)
	for _, pair := range args {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Scale parameters must be provided as NAME=VALUE pairs (%s is invalid)", pair)
		}
		key := parts[0]
		value := parts[1]
		kvargs[strings.ToLower(key)] = value
	}

	minval, found := kvargs["min"]

	if found {
		minint64val, err := strconv.ParseInt(minval, 10, 64)
		if err != nil {
			return errors.New("could not parse min count value")
		}
		minintval := int(minint64val)
		newcfg.MinCount = &minintval
		delete(kvargs, "min")
	}

	maxval, found := kvargs["max"]

	if found {
		maxint64val, err := strconv.ParseInt(maxval, 10, 64)
		if err != nil {
			return errors.New("could not parse max count value")
		}
		maxintval := int(maxint64val)
		newcfg.MaxCount = &maxintval
		delete(kvargs, "max")
	}

	if len(kvargs) != 0 {
		unusedkeys := ""
		for k := range kvargs {
			if unusedkeys == "" {
				unusedkeys = k
			} else {
				unusedkeys = unusedkeys + ", " + k
			}
		}
		return errors.New("unrecognised parameters in command:" + unusedkeys)
	}

	cfg, err := apiClient.UpdateAutoscaleConfig(ctx, newcfg)
	if err != nil {
		return err
	}

	printScaleConfig(ctx, cfg)

	return nil
}

func runAutoscaleShow(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	cfg, err := apiClient.AppAutoscalingConfig(ctx, appName)
	if err != nil {
		return err
	}

	printScaleConfig(ctx, cfg)

	return nil
}

func printScaleConfig(ctx context.Context, cfg *api.AutoscalingConfig) {
	io := iostreams.FromContext(ctx)

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, cfg)
		return
	}

	var mode string

	if !cfg.Enabled {
		mode = "Disabled"
	} else {
		mode = "Enabled"
	}

	fmt.Fprintf(io.Out, "%15s: %s\n", "Autoscaling", mode)
	if cfg.Enabled {
		fmt.Fprintf(io.Out, "%15s: %d\n", "Min Count", cfg.MinCount)
		fmt.Fprintf(io.Out, "%15s: %d\n", "Max Count", cfg.MaxCount)
	}
}

func printMachinesAutoscalingBanner() {
	fmt.Printf(`
Configuring autoscaling via 'flyctl autoscale' is supported only for apps running on Nomad platform.
Refer to this post for details on how to enable autoscaling for Apps V2:
https://community.fly.io/t/increasing-apps-v2-availability/12357
`)
}
