package regions

import (
	"context"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "V1 APPS ONLY: Manage regions"
		long  = `V1 APPS ONLY (except 'regions list'): Configure the region placement rules for an application.`
	)
	cmd := command.New("regions", short, long, nil)
	cmd.AddCommand(
		newRegionsAdd(),
		newRegionsRemove(),
		newRegionsSet(),
		newRegionsBackup(),
		newRegionsList(),
	)
	return cmd
}

func newRegionsAdd() *cobra.Command {
	const (
		short = `V1 APPS ONLY: Allow the app to run in the provided regions`
		long  = `V1 APPS ONLY: Allow the app to run in one or more regions`
	)
	cmd := command.New("add REGION [REGION...]", short, long, runRegionsAdd,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MinimumNArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.Yes(),
		flag.JSONOutput(),
		flag.String{Name: "group", Description: "The process group to add the region to"},
	)
	return cmd
}

func newRegionsRemove() *cobra.Command {
	const (
		short = `V1 APPS ONLY: Prevent the app from running in the provided regions`
		long  = `V1 APPS ONLY: Prevent the app from running in the provided regions`
	)
	cmd := command.New("remove REGION [REGION...]", short, long, runRegionsRemove,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MinimumNArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.Yes(),
		flag.JSONOutput(),
		flag.String{Name: "group", Description: "The process group to add the region to"},
	)
	return cmd
}

func newRegionsSet() *cobra.Command {
	const (
		short = `V1 APPS ONLY: Sets the region pool with provided regions`
		long  = `V1 APPS ONLY: Sets the region pool with provided regions`
	)
	cmd := command.New("set REGION [REGION...]", short, long, runRegionsSet,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MinimumNArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.Yes(),
		flag.JSONOutput(),
		flag.String{Name: "group", Description: "The process group to add the region to"},
	)
	return cmd
}

func newRegionsBackup() *cobra.Command {
	const (
		short = `V1 APPS ONLY: Sets the backup region pool with provided regions`
		long  = `V1 APPS ONLY: Sets the backup region pool with provided regions`
	)
	cmd := command.New("backup REGION [REGION...]", short, long, runRegionsBackup,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.MinimumNArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.Yes(),
		flag.JSONOutput(),
	)
	return cmd
}

func newRegionsList() *cobra.Command {
	const (
		short = `Shows the list of regions the app is allowed to run in`
		long  = `Shows the list of regions the app is allowed to run in`
	)
	cmd := command.New("list", short, long, runRegionsList,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
	)
	return cmd
}

func isV2(ctx context.Context) (bool, error) {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return false, err
	}
	return app.PlatformVersion != "nomad", nil
}

func runRegionsList(ctx context.Context) error {
	switch v2, err := isV2(ctx); {
	case err != nil:
		return err
	case v2:
		return v2RunRegionsList(ctx)
	default:
		return v1RunRegionsList(ctx)
	}
}

func runRegionsAdd(ctx context.Context) error {
	switch v2, err := isV2(ctx); {
	case err != nil:
		return err
	case v2:
		return v2RunRegionsAdd(ctx)
	default:
		return v1RunRegionsAdd(ctx)
	}
}
