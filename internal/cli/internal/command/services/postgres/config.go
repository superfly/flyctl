package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/r3labs/diff"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newConfig() (cmd *cobra.Command) {
	// TODO - Add better top level docs.
	const (
		long = `View and manage Postgres configuration.
`
		short = "View and manage Postgres configuration."
	)

	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newConfigView(),
		newConfigUpdate(),
	)

	return
}

// pgSettingMap maps the command-line argument to the actual pgParameter.
// This also acts as a whitelist as far as what's configurable via flyctl and
// can be expanded on as needed.
var pgSettingMap = map[string]string{
	"wal-level":                  "wal_level",
	"max-connections":            "max_connections",
	"log-statement":              "log_statement",
	"log-min-duration-statement": "log_min_duration_statement",
	"log-duration":               "log_duration",
}

func newConfigView() (cmd *cobra.Command) {
	const (
		long = `View your Postgres configuration
`
		short = "View your Postgres configuration"
		usage = "view"
	)

	cmd = command.New(usage, short, long, runConfigView,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runConfigView(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgCmd, err := newPostgresCmd(ctx, app)
	if err != nil {
		return err
	}

	var settings []string
	for _, k := range pgSettingMap {
		settings = append(settings, k)
	}

	resp, err := pgCmd.viewSettings(settings)
	if err != nil {
		return err
	}

	pendingRestart := false
	rows := make([][]string, 0, len(resp.Settings))
	for _, setting := range resp.Settings {
		desc := setting.Desc
		switch setting.VarType {
		case "enum":
			e := strings.Join(setting.EnumVals, ", ")
			desc = fmt.Sprintf("%s [%s]", desc, e)
		case "integer", "real":
			desc = fmt.Sprintf("%s (%s, %s)", desc, setting.MinVal, setting.MaxVal)
		case "bool":
			desc = fmt.Sprintf("%s [on, off]", desc)

		}

		value := setting.Setting
		restart := fmt.Sprint(setting.PendingRestart)
		if setting.PendingRestart {
			pendingRestart = true
			restart = colorize.Bold(restart)
		}
		if setting.PendingChange != "" {
			p := colorize.Bold(fmt.Sprintf("(%s)", setting.PendingChange))
			value = fmt.Sprintf("%s -> %s", value, p)
		}
		rows = append(rows, []string{
			strings.Replace(setting.Name, "_", "-", -1),
			value,
			desc,
			restart,
		})
	}
	_ = render.Table(io.Out, "", rows, "Name", "Value", "Description", "Pending Restart")

	if pendingRestart {
		fmt.Fprintln(io.Out, colorize.Yellow("Some changes are awaiting a restart!"))
		fmt.Fprintln(io.Out, colorize.Yellow(fmt.Sprintf("To apply changes, run: `DEV=1 fly services postgres restart --app %s`", appName)))
	}

	return nil
}

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long = `Update Postgres configuration.
`
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
		flag.String{
			Name:        "max-connections",
			Description: "Sets the maximum number of concurrent connections.",
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
			Name:        "log-duration",
			Description: "Logs the duration of each completed SQL statement.",
		},
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Will automatically confirm changes without an interactive prompt.",
		},
	)

	return
}

func runConfigUpdate(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgCmd, err := newPostgresCmd(ctx, app)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// Identify requested configuration changes.
	rChanges := map[string]string{}
	keys := []string{}
	for key := range pgSettingMap {
		val := flag.GetString(ctx, key)
		if val != "" {
			rChanges[pgSettingMap[key]] = val
			keys = append(keys, pgSettingMap[key])
		}
	}

	if len(rChanges) == 0 {
		return fmt.Errorf("no changes were specified")
	}

	// Pull existing configuration
	settings, err := pgCmd.viewSettings(keys)
	if err != nil {
		return err
	}

	// Verfiy that input values are within acceptible ranges.
	// Stolon does not verify this, so we need to do it here.
	for k, v := range rChanges {
		for _, setting := range settings.Settings {
			if setting.Name == k {
				if err = validateConfigValue(setting, k, v); err != nil {
					return err
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
	changelog, _ := diff.Diff(oValues, rChanges)
	if len(changelog) == 0 {
		return fmt.Errorf("no changes to apply")
	}

	restartRequired := false

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
	_ = render.Table(io.Out, "", rows, "Name", "Value", "Target value", "Restart Required")

	if !flag.GetBool(ctx, "auto-confirm") {
		const msg = "Are you sure you want to apply these changes?"

		switch confirmed, err := prompt.Confirmf(ctx, msg); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("auto-confirm flag must be specified when not running interactively")
		default:
			return err
		}
	}

	fmt.Fprintln(io.Out, "Performing update...")
	err = pgCmd.updateSettings(rChanges)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Update complete!")

	if restartRequired {
		fmt.Fprintln(io.Out, colorize.Yellow("Please note that some of your changes will require a cluster restart before they will be applied."))
		fmt.Fprintln(io.Out, colorize.Yellow("To review the state of your changes, run: `DEV=1 fly services postgres config view`"))
	}

	return nil
}

func isRestartRequired(pgSettings *pgSettings, name string) bool {
	for _, s := range pgSettings.Settings {
		if s.Name == name {
			if s.Context == "postmaster" {
				return true
			}
		}
	}

	return false
}

func validateConfigValue(setting pgSetting, key, val string) error {
	switch setting.VarType {
	case "enum":
		for _, enumVal := range setting.EnumVals {
			if enumVal == val {
				return nil
			}
		}
		return fmt.Errorf("Invalid value specified for %s. Received: %s, Accepted values: [%s]", key, val, strings.Join(setting.EnumVals, ", "))
	case "integer", "real":
		min, err := strconv.Atoi(setting.MinVal)
		if err != nil {
			return err
		}
		max, err := strconv.Atoi(setting.MaxVal)
		if err != nil {
			return err
		}

		v, err := strconv.Atoi(val)
		if err != nil || v < min || v > max {
			return fmt.Errorf("Invalid value specified for %s. (Received: %s, Accepted range: (%s, %s)", key, val, setting.MinVal, setting.MaxVal)
		}
	case "bool":
		if val != "on" && val != "off" {
			return fmt.Errorf("Invalid value specified for %s. (Received: %s, Accepted values: [on, off]", key, val)
		}

	}

	return nil
}
