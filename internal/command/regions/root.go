package regions

import (
	"context"
	"fmt"

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
	cmd.Hidden = true
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
		flag.AppConfig(),
		flag.Yes(),
		flag.JSONOutput(),
		flag.String{Name: "group", Description: "The process group to add the region to"},
	)
	cmd.Hidden = true
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
		flag.AppConfig(),
		flag.Yes(),
		flag.JSONOutput(),
		flag.String{Name: "group", Description: "The process group to add the region to"},
	)
	cmd.Hidden = true
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
		flag.AppConfig(),
		flag.Yes(),
		flag.JSONOutput(),
		flag.String{Name: "group", Description: "The process group to add the region to"},
	)
	cmd.Hidden = true
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
		flag.AppConfig(),
		flag.Yes(),
		flag.JSONOutput(),
	)
	cmd.Hidden = true
	return cmd
}

func newRegionsList() *cobra.Command {
	const (
		short = `Shows the list of regions the app is allowed to run in`
		long  = `Shows the list of regions the app is allowed to run in`
	)
	cmd := command.New("list", short, long, v2RunRegionsList,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	return cmd
}

func runRegionsAdd(ctx context.Context) error {
	return fmt.Errorf("This command is no longer supported; use fly scale count to scale the number of Machines in a region. See https://fly.io/docs/launch/scale-count/.")
}

func runRegionsRemove(ctx context.Context) error {
	return fmt.Errorf("This command is no longer supported; use fly scale count to scale the number of Machines in a region. See https://fly.io/docs/launch/scale-count/.")
}

func runRegionsSet(ctx context.Context) error {
	return fmt.Errorf("This command is no longer supported; use fly scale count to scale the number of Machines in a region. See https://fly.io/docs/launch/scale-count/.")
}

func runRegionsBackup(ctx context.Context) error {
	return fmt.Errorf("This command is no longer supported; use fly scale count to scale the number of Machines in a region. See https://fly.io/docs/launch/scale-count/.")
}
