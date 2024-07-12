package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/r3labs/diff"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long  = `Update Fly Postgres configuration.`
		short = "Update Fly Postgres configuration."
		usage = "update"
	)

	cmd = command.New(usage, short, long, runConfigUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Detach(),
		flag.String{
			Name:        "max-connections",
			Description: "Sets the maximum number of concurrent connections.",
		},
		flag.String{
			Name:        "shared-buffers",
			Description: "Sets the amount of memory the database server uses for shared memory buffers",
		},
		flag.String{
			Name:        "wal-level",
			Description: "Sets the level of information written to the WAL. (minimal, replica, logical).",
		},
		flag.String{
			Name:        "max-wal-senders",
			Description: "Maximum number of concurrent connections from standby servers or streaming backup clients. (0 disables replication)",
		},
		flag.String{
			Name:        "max-replication-slots",
			Description: "Specifies the maximum number of replication slots. This should typically match max_wal_senders.",
		},
		flag.String{
			Name:        "log-statement",
			Description: "Sets the type of statements logged. (none, ddl, mod, all)",
		},
		flag.String{
			Name:        "log-min-duration-statement",
			Description: "Sets the minimum execution time above which all statements will be logged. (ms)",
		},
		flag.String{
			Name:        "shared-preload-libraries",
			Description: "Sets the shared libraries to preload. (comma separated string)",
		},
		flag.String{
			Name:        "work-mem",
			Description: "Sets the maximum amount of memory each Postgres query can use",
		},
		flag.String{
			Name:        "maintenance-work-mem",
			Description: "Sets the maximum amount of memory used for maintenance operations like ALTER TABLE, CREATE INDEX, and VACUUM",
		},
		flag.Bool{
			Name:        "force",
			Description: "Skips pg-setting value verification.",
		},
		flag.Yes(),
	)

	return
}

func runConfigUpdate(ctx context.Context) error {
	var (
		client  = flyutil.ClientFromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}
	return runMachineConfigUpdate(ctx, app)
}

func runMachineConfigUpdate(ctx context.Context, app *fly.AppCompact) error {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		autoConfirm = flag.GetBool(ctx, "yes")

		MinPostgresHaVersion         = "0.0.33"
		MinPostgresStandaloneVersion = "0.0.7"
		MinPostgresFlexVersion       = "0.0.6"
	)

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc()
	if err != nil {
		return fmt.Errorf("machines could not be retrieved")
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	manager := flypg.StolonManager
	if IsFlex(leader) {
		manager = flypg.ReplicationManager
	}

	requiresRestart := false

	switch manager {
	case flypg.ReplicationManager:
		requiresRestart, err = updateFlexConfig(ctx, app, leader.PrivateIP)
		if err != nil {
			return err
		}
	default:
		requiresRestart, err = updateStolonConfig(ctx, app, leader.PrivateIP)
		if err != nil {
			return err
		}

	}

	if requiresRestart {
		if !autoConfirm {
			fmt.Fprintln(io.Out, colorize.Yellow("Please note that some of your changes will require a cluster restart before they will be applied."))

			switch confirmed, err := prompt.Confirm(ctx, "Restart cluster now?"); {
			case err == nil:
				if !confirmed {
					return nil
				}
			case prompt.IsNonInteractive(err):
				return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
			default:
				return err
			}
		}

		// Ensure leases are released before we issue restart.
		releaseLeaseFunc()
		if err := machinesRestart(ctx, &fly.RestartMachineInput{}); err != nil {
			return err
		}
	}

	return nil
}

func updateStolonConfig(ctx context.Context, app *fly.AppCompact, leaderIP string) (bool, error) {
	io := iostreams.FromContext(ctx)

	restartRequired, changes, err := resolveConfigChanges(ctx, app, flypg.StolonManager, leaderIP)
	if err != nil {
		return false, err
	}

	fmt.Fprintln(io.Out, "Performing update...")
	cmd, err := flypg.NewCommand(ctx, app)
	if err != nil {
		return false, err
	}

	err = cmd.UpdateSettings(ctx, leaderIP, changes)
	if err != nil {
		return false, err
	}
	fmt.Fprintln(io.Out, "Update complete!")

	return restartRequired, nil
}

func updateFlexConfig(ctx context.Context, app *fly.AppCompact, leaderIP string) (bool, error) {
	var (
		io     = iostreams.FromContext(ctx)
		dialer = agent.DialerFromContext(ctx)
	)

	restartRequired, changes, err := resolveConfigChanges(ctx, app, flypg.ReplicationManager, leaderIP)
	if err != nil {
		return false, err
	}

	fmt.Fprintln(io.Out, "Performing update...")
	leaderClient := flypg.NewFromInstance(leaderIP, dialer)

	// Push configuration settings to consul.
	if err := leaderClient.UpdateSettings(ctx, changes); err != nil {
		return false, err
	}

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return false, err
	}

	// Sync configuration settings for each node. This should be safe to apply out-of-order.
	for _, machine := range machines {
		if machine.Config.Env["IS_BARMAN"] != "" {
			continue
		}

		client := flypg.NewFromInstance(machine.PrivateIP, dialer)

		// Pull configuration settings down from Consul for each node and reload the config.
		err := client.SyncSettings(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to sync configuration on %s: %s", machine.ID, err)
		}
	}
	fmt.Fprintln(io.Out, "Update complete!")

	return restartRequired, nil
}

func resolveConfigChanges(ctx context.Context, app *fly.AppCompact, manager string, leaderIP string) (bool, map[string]string, error) {
	var (
		io     = iostreams.FromContext(ctx)
		dialer = agent.DialerFromContext(ctx)

		force       = flag.GetBool(ctx, "force")
		autoConfirm = flag.GetYes(ctx)
	)

	// Identify requested configuration changes.
	changes := map[string]string{}
	keys := []string{}
	for key := range pgSettings {
		val := flag.GetString(ctx, key)
		if val != "" {
			changes[pgSettings[key]] = val
			keys = append(keys, pgSettings[key])
		}
	}

	restartRequired := false
	if !force {
		// Query PG settings
		pgclient := flypg.NewFromInstance(leaderIP, dialer)

		settings, err := pgclient.ViewSettings(ctx, keys, manager)
		if err != nil {
			return false, nil, err
		}

		if len(changes) == 0 {
			return false, nil, fmt.Errorf("no changes were specified")
		}

		changelog, err := resolveChangeLog(ctx, changes, settings)
		if err != nil {
			return false, nil, err
		}
		if len(changelog) == 0 {
			return false, nil, fmt.Errorf("no changes to apply")
		}

		rows := make([][]string, 0, len(changelog))
		for _, change := range changelog {
			requiresRestart := isRestartRequired(settings, change.Path[len(change.Path)-1])
			if requiresRestart {
				restartRequired = true
			}

			name := strings.ReplaceAll(change.Path[len(change.Path)-1], "_", "-")
			rows = append(rows, []string{
				name,
				fmt.Sprint(change.From),
				fmt.Sprint(change.To),
				fmt.Sprint(requiresRestart),
			})
		}
		render.Table(io.Out, "", rows, "Name", "Value", "Target value", "Restart Required")

		if !autoConfirm {
			const msg = "Are you sure you want to apply these changes?"

			switch confirmed, err := prompt.Confirmf(ctx, msg); {
			case err == nil:
				if !confirmed {
					return false, nil, nil
				}
			case prompt.IsNonInteractive(err):
				return false, nil, prompt.NonInteractiveError("yes flag must be specified when not running interactively")
			default:
				return false, nil, err
			}
		}
	}

	return restartRequired, changes, nil
}

func resolveChangeLog(ctx context.Context, changes map[string]string, settings *flypg.PGSettings) (diff.Changelog, error) {
	// Verify that input values are within acceptible ranges.
	// Stolon does not verify this, so we need to do it here.
	for k, v := range changes {
		for _, setting := range settings.Settings {
			if setting.Name == k {
				if err := validateConfigValue(setting, k, v); err != nil {
					return nil, err
				}
			}
		}
	}

	// Construct a map of the active configuration settings so we can compare.
	oValues := map[string]string{}
	for _, setting := range settings.Settings {
		oValues[setting.Name] = setting.Setting
	}

	// Calculate diff
	return diff.Diff(oValues, changes)
}

func isRestartRequired(pgSettings *flypg.PGSettings, name string) bool {
	for _, s := range pgSettings.Settings {
		if s.Name == name {
			if s.Context == "postmaster" {
				return true
			}
		}
	}

	return false
}

func validateConfigValue(setting flypg.PGSetting, key, val string) error {
	switch setting.VarType {
	case "enum":
		for _, enumVal := range setting.EnumVals {
			if enumVal == val {
				return nil
			}
		}
		return fmt.Errorf("invalid value specified for %s. Received: %s, Accepted values: [%s]", key, val, strings.Join(setting.EnumVals, ", "))
	case "integer":
		min, err := strconv.Atoi(setting.MinVal)
		if err != nil {
			return err
		}
		max, err := strconv.Atoi(setting.MaxVal)
		if err != nil {
			return err
		}

		v, err := strconv.Atoi(val)
		if err != nil {
			return err
		}

		if v < min || v > max {
			return fmt.Errorf("invalid value specified for %s. (Received: %s, Accepted range: (%s, %s)", key, val, setting.MinVal, setting.MaxVal)
		}
	case "real":
		min, err := strconv.ParseFloat(setting.MinVal, 32)
		if err != nil {
			return err
		}

		max, err := strconv.ParseFloat(setting.MaxVal, 32)
		if err != nil {
			return err
		}

		v, err := strconv.ParseFloat(val, 32)
		if err != nil {
			return err
		}

		if v < min || v > max {
			return fmt.Errorf("invalid value specified for %s. (Received: %s, Accepted range: (%.1f, %.1f)", key, val, min, max)
		}
	case "bool":
		if val != "on" && val != "off" {
			return fmt.Errorf("invalid value specified for %s. (Received: %s, Accepted values: [on, off]", key, val)
		}
	}

	return nil
}
