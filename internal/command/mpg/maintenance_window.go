package mpg

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

var weekdays = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

func newMaintenanceWindow() *cobra.Command {
	const (
		short = "Show or change a cluster's weekly maintenance window"
		long  = `Show or change the weekly window during which a Managed Postgres cluster
may undergo maintenance. Times are in the cluster's regional timezone.`
	)

	cmd := command.New("maintenance-window", short, long, nil)
	cmd.Aliases = []string{"maintenance"}

	cmd.AddCommand(
		newMaintenanceWindowShow(),
		newMaintenanceWindowSet(),
	)

	return cmd
}

func newMaintenanceWindowShow() *cobra.Command {
	const (
		long  = `Show the weekly maintenance window of a Managed Postgres cluster.`
		short = "Show an MPG cluster's maintenance window."
		usage = "show [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runMaintenanceWindowShow,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runMaintenanceWindowShow(ctx context.Context) error {
	cluster, err := maintenanceWindowCluster(ctx)
	if err != nil {
		return err
	}

	return cmdv2.RunMaintenanceWindowShow(ctx, cluster.Id)
}

func newMaintenanceWindowSet() *cobra.Command {
	const (
		long = `Change the weekly maintenance window of a Managed Postgres cluster.
Only the flags you pass are changed; the rest of the window keeps its current
value. The start hour is in the cluster's regional timezone, shown by
'fly mpg maintenance-window show'.`
		short = "Change an MPG cluster's maintenance window."
		usage = "set [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runMaintenanceWindowSet,
		command.RequireSession,
		requireMacaroonToken,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.String{
			Name:        "day",
			Description: "Day of the week the window starts on, e.g. sunday",
		},
		flag.Int{
			Name:        "start-hour",
			Description: "Hour the window starts, 0-23, in the cluster's timezone",
		},
		flag.Int{
			Name:        "duration",
			Description: "Length of the window in hours, 3-8",
		},
	)

	return cmd
}

func runMaintenanceWindowSet(ctx context.Context) error {
	// Validate the flags before resolving the cluster so a bad invocation
	// fails without prompting for a cluster first.
	req, err := maintenanceWindowRequestFromFlags(ctx)
	if err != nil {
		return err
	}

	cluster, err := maintenanceWindowCluster(ctx)
	if err != nil {
		return err
	}

	return cmdv2.RunMaintenanceWindowSet(ctx, cluster.Id, req)
}

func maintenanceWindowCluster(ctx context.Context) (*mpg.Cluster, error) {
	cluster, _, err := ClusterFromArgOrSelect(ctx, flag.FirstArg(ctx), "")
	if err != nil {
		return nil, err
	}
	if cluster.Version == mpg.VersionV1 {
		return nil, fmt.Errorf("maintenance windows are not supported for this cluster")
	}

	return cluster, nil
}

// maintenanceWindowRequestFromFlags builds the update request from only the
// flags the user passed, so that --start-hour 0 is sent and unset fields keep
// their current value. The duration's bounds are left to the API, which owns
// them.
func maintenanceWindowRequestFromFlags(ctx context.Context) (flaps.UpdateManagedPostgresMaintenanceWindowRequest, error) {
	var req flaps.UpdateManagedPostgresMaintenanceWindowRequest

	if flag.IsSpecified(ctx, "day") {
		day := strings.ToLower(strings.TrimSpace(flag.GetString(ctx, "day")))
		if !slices.Contains(weekdays, day) {
			return req, fmt.Errorf("--day must be a day of the week (%s), got %q", strings.Join(weekdays, ", "), flag.GetString(ctx, "day"))
		}
		req.Day = &day
	}

	if flag.IsSpecified(ctx, "start-hour") {
		hour := flag.GetInt(ctx, "start-hour")
		if hour < 0 || hour > 23 {
			return req, fmt.Errorf("--start-hour must be between 0 and 23, got %d", hour)
		}
		req.StartHour = &hour
	}

	if flag.IsSpecified(ctx, "duration") {
		duration := flag.GetInt(ctx, "duration")
		req.DurationHours = &duration
	}

	if req.Day == nil && req.StartHour == nil && req.DurationHours == nil {
		return req, errors.New("specify at least one of --day, --start-hour or --duration")
	}

	return req, nil
}
