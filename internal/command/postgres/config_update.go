package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/r3labs/diff"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long  = `Update Postgres configuration.`
		short = "Update Postgres configuration."
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
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
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

	switch app.PlatformVersion {
	case "machines":
		return runMachineConfigUpdate(ctx, app)
	case "nomad":
		return runNomadConfigUpdate(ctx, app)
	default:
		return fmt.Errorf("unknown platform version")
	}
}

func runMachineConfigUpdate(ctx context.Context, app *api.AppCompact) error {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		autoConfirm = flag.GetBool(ctx, "yes")
	)

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc(ctx, machines)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved")
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	requiresRestart, err := updateStolonConfig(ctx, app, leader.PrivateIP)
	if err != nil {
		return err
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
		releaseLeaseFunc(ctx, machines)
		if err := machinesRestart(ctx, &api.RestartMachineInput{}); err != nil {
			return err
		}
	}

	return nil
}

func runNomadConfigUpdate(ctx context.Context, app *api.AppCompact) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()

		autoConfirm = flag.GetBool(ctx, "yes")
	)

	client := client.FromContext(ctx).API()

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	pgInstances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", app.Name, err)
	}
	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", app.Name)
	}

	leaderIP, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	requiresRestart, err := updateStolonConfig(ctx, app, leaderIP)
	if err != nil {
		return err
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

		if err := nomadRestart(ctx, app); err != nil {
			return err
		}
	}

	return nil
}

func updateStolonConfig(ctx context.Context, app *api.AppCompact, leaderIP string) (bool, error) {
	var (
		io     = iostreams.FromContext(ctx)
		dialer = agent.DialerFromContext(ctx)

		force       = flag.GetBool(ctx, "force")
		autoConfirm = flag.GetBool(ctx, "yes")
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
		settings, err := pgclient.SettingsView(ctx, keys)
		if err != nil {
			return false, err
		}
		if len(changes) == 0 {
			return false, fmt.Errorf("no changes were specified")
		}

		changelog, err := resolveChangeLog(ctx, changes, settings)
		if err != nil {
			return false, err
		}
		if len(changelog) == 0 {
			return false, fmt.Errorf("no changes to apply")
		}

		rows := make([][]string, 0, len(changelog))
		for _, change := range changelog {
			requiresRestart := isRestartRequired(settings, change.Path[len(change.Path)-1])
			if requiresRestart {
				restartRequired = true
			}

			name := strings.Replace(change.Path[len(change.Path)-1], "_", "-", -1)
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
					return false, nil
				}
			case prompt.IsNonInteractive(err):
				return false, prompt.NonInteractiveError("auto-confirm flag must be specified when not running interactively")
			default:
				return false, err
			}
		}
	}

	cmd, err := flypg.NewCommand(ctx, app)
	if err != nil {
		return false, err
	}

	fmt.Fprintln(io.Out, "Performing update...")

	err = cmd.UpdateSettings(ctx, leaderIP, changes)
	if err != nil {
		return false, err
	}
	fmt.Fprintln(io.Out, "Update complete!")

	return restartRequired, nil
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
